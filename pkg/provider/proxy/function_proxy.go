/*
Based on "github.com/openfaas/faas-provider/proxy"
*/
package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/gorilla/mux"
	"github.com/openfaas/faas-provider/httputil"
	"github.com/openfaas/faas-provider/types"

	"github.com/google/uuid"

	"github.gatech.edu/faasedge/fecore/pkg/provider/handlers"
	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

const (
	watchdogPort       = "8080"
	defaultContentType = "text/plain"
)

func NewHandlerFunc(config types.FaaSConfig, resolver *handlers.InvokeResolver, fs *handlers.FunctionStore) http.HandlerFunc {
	if resolver == nil {
		panic("NewHandlerFunc: empty proxy handler resolver, cannot be nil")
	}

	/* Old method: Create proxyClient from OpenFaaS config */
	// proxyClient := proxy.NewProxyClientFromConfig(config)

	/* New method: Create client using retryableHttp pkg */
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 632 // Gives ~10 secs of retry
	/* Uncomment below to use dynamic backoff time between request attempts */
	// retryClient.Backoff(time.Duration(1)*time.Millisecond, time.Duration(25)*time.Millisecond, 25, nil)
	/* Always wait 5 ms backoff time between request attempts */
	retryClient.RetryWaitMin = time.Duration(5) * time.Millisecond
	retryClient.RetryWaitMax = time.Duration(5) * time.Millisecond
	retryClient.Logger = nil
	proxyClient := retryClient.StandardClient()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}

		switch r.Method {
		case http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodGet,
			http.MethodOptions,
			http.MethodHead:
			proxyRequest(w, r, proxyClient, resolver, fs)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// proxyRequest handles the actual resolution of and then request to the function service.
func proxyRequest(w http.ResponseWriter, originalReq *http.Request, proxyClient *http.Client, resolver *handlers.InvokeResolver, fs *handlers.FunctionStore) {
	/* Begin function replica setup */
	fnSetupStart := time.Now()
	ctx := originalReq.Context()

	pathVars := mux.Vars(originalReq)
	functionName := pathVars["name"]
	requestID := functionName + "_" + uuid.New().String()
	defer timec.RecordDuration("[function_proxy.go/proxyRequest()] <requestID="+requestID+">", time.Now())
	if functionName == "" {
		httputil.Errorf(w, http.StatusBadRequest, "Provide function name in the request path")
		return
	}

	reqStartupType := originalReq.Header.Get("startupType")
	reqContainerType := originalReq.Header.Get("containerType")

	functionAddr, startupType, containerType, replicaName, resolveErr := resolver.Resolve(functionName, requestID, reqStartupType, reqContainerType)
	if resolveErr != nil {
		timec.LogEvent("function_proxy/proxyRequest", fmt.Sprintf("Resolver error: No endpoints for %s: %s", functionName, resolveErr.Error()), 1)
		httputil.Errorf(w, http.StatusServiceUnavailable, "No endpoints available for: %s.", functionName)
		return
	}

	var proxyReq *http.Request

	var err error
	proxyReq, err = buildProxyRequest(originalReq, functionAddr, pathVars["params"])
	if err != nil {
		httputil.Errorf(w, http.StatusInternalServerError, "Failed to resolve service: %s.", functionName)
		return
	}

	if proxyReq.Body != nil {
		defer proxyReq.Body.Close()
	}

	fnSetupTime := time.Since(fnSetupStart).Milliseconds()
	timec.LogEvent("function_proxy/proxyRequest", fmt.Sprintf("<requestID=%s> Setup for %s took %d ms", requestID, replicaName, fnSetupTime), 2)
	/* End function replica setup */

	var response *http.Response

	fnExecStart := time.Now()

	ip := strings.Split(functionAddr.Host, ":")[0]

	/* Attempt connection to function replica; retryableHttp will keep trying
	 * until it connects or times out */
	response, err = proxyClient.Do(proxyReq.WithContext(ctx))

	/* Connection timed out or we got a bad status from replica */
	if err != nil || response.StatusCode != 200 {
		timec.LogEvent("function_proxy/proxyRequest", fmt.Sprintf("<requestID=%s>  Connecting to function %s at %s failed. Status code: %d; Error: %s", requestID, functionName, ip, response.StatusCode, err), 1)
		httputil.Errorf(w, http.StatusInternalServerError, "Can't reach service for '%s'", functionName)
		return
	}

	/* Connection was successful; report time time taken to exec function */
	fnExecTime := time.Since(fnExecStart).Milliseconds()
	timec.LogEvent("function_proxy/proxyRequest", fmt.Sprintf("<requestID=%s> Exec for %s took %d ms", requestID, replicaName, fnExecTime), 2)

	timec.LogEvent("function_proxy/proxyRequest", fmt.Sprintf("<requestID=%s> Success connecting to function %s (setupTime=%d ; execTime=%d)", requestID, functionName, fnSetupTime, fnExecTime), 2)

	// proxyElapsed := time.Since(proxyStart)
	// timec.RecordDuration("(function_proxy.go) proxyRequest() : [ProxyRequestTime] <requestID="+requestID+", startupType="+startupType+", retries="+strconv.Itoa(connectionRetries)+", connectTime="+strconv.Itoa(int(totalConnectTime))+" ms>", proxyStart)

	if response.Body != nil {
		defer response.Body.Close()
	}

	fs.RecordInvocationTime(fnSetupTime+fnExecTime, startupType)
	fs.UpdateFunctionStats(functionName, containerType, replicaName, fnSetupTime, fnExecTime, startupType)
	err = fs.UpdateReplicaStatusInactive(functionName, replicaName, requestID)
	if err != nil {
		timec.LogEvent("function_proxy/proxyRequest", fmt.Sprintf("<requestID=%s> Unable to update replica status to inactive: %s\n", requestID, err.Error()), 1)
		httputil.Errorf(w, http.StatusInternalServerError, "Can't reach service for '%s'", functionName)
		return
	}

	clientHeader := w.Header()
	copyHeaders(clientHeader, &response.Header)
	w.Header().Set("Content-Type", getContentType(originalReq.Header, response.Header))
	w.Header().Set("Request-ID", requestID)
	w.Header().Set("Startup-Type", startupType)
	w.Header().Set("Container-Type", containerType)
	w.Header().Set("Setup-Time", strconv.Itoa(int(fnSetupTime)))
	w.Header().Set("Container-Name", replicaName)

	/* This is the invocation time as reported by the client inside the function container */
	invocation_elapsed := response.Header.Get("invocation-elapsed")
	if invocation_elapsed != "" {
		timec.LogEvent("function_proxy/proxyRequest", fmt.Sprintf("<requestID=%s> invocation-elapsed: %s ms", requestID, invocation_elapsed), 2)
	}

	/* Debug: function loader may return misc. timing stats gathered
	 * internally to help understand what is going on inside
	 * replica sandbox */
	// misc_stats := response.Header.Get("misc-stats")
	// if invocation_elapsed != "" {
	// 	timec.LogEvent("function_proxy/proxyRequest", fmt.Sprintf("<requestID=%s> misc-stats: %s", requestID, misc_stats))
	// }

	w.WriteHeader(response.StatusCode)
	if response.Body != nil {
		io.Copy(w, response.Body)
	}
}

// buildProxyRequest creates a request object for the proxy request, it will ensure that
// the original request headers are preserved as well as setting openfaas system headers
func buildProxyRequest(originalReq *http.Request, baseURL url.URL, extraPath string) (*http.Request, error) {

	host := baseURL.Host
	if baseURL.Port() == "" {
		host = baseURL.Host + ":" + watchdogPort
	}

	url := url.URL{
		Scheme:   baseURL.Scheme,
		Host:     host,
		Path:     extraPath,
		RawQuery: originalReq.URL.RawQuery,
	}

	upstreamReq, err := http.NewRequest(originalReq.Method, url.String(), nil)
	if err != nil {
		return nil, err
	}
	copyHeaders(upstreamReq.Header, &originalReq.Header)

	if len(originalReq.Host) > 0 && upstreamReq.Header.Get("X-Forwarded-Host") == "" {
		upstreamReq.Header["X-Forwarded-Host"] = []string{originalReq.Host}
	}
	if upstreamReq.Header.Get("X-Forwarded-For") == "" {
		upstreamReq.Header["X-Forwarded-For"] = []string{originalReq.RemoteAddr}
	}

	if originalReq.Body != nil {
		upstreamReq.Body = originalReq.Body
	}

	return upstreamReq, nil
}

// copyHeaders clones the header values from the source into the destination.
func copyHeaders(destination http.Header, source *http.Header) {
	for k, v := range *source {
		vClone := make([]string, len(v))
		copy(vClone, v)
		destination[k] = vClone
	}
}

// getContentType resolves the correct Content-Type for a proxied function.
func getContentType(request http.Header, proxyResponse http.Header) (headerContentType string) {
	responseHeader := proxyResponse.Get("Content-Type")
	requestHeader := request.Get("Content-Type")

	if len(responseHeader) > 0 {
		headerContentType = responseHeader
	} else if len(requestHeader) > 0 {
		headerContentType = requestHeader
	} else {
		headerContentType = defaultContentType
	}

	return headerContentType
}

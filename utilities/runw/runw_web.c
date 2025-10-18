/* Web server based on sample code found here: https://bruinsslot.jp/post/simple-http-webserver-in-c/ */

#define _GNU_SOURCE

#include <wasmedge/wasmedge.h>
#include <stdio.h>
#include <errno.h>
#include <error.h>
#include <stdlib.h>
#include <signal.h>
#include <sched.h>
//#include <math.h>

#include <linux/limits.h>
#include <fcntl.h>
#include <unistd.h>

#include <time.h>
#include <netdb.h>
#include <ifaddrs.h>

#include <arpa/inet.h>
#include <string.h>
#include <sys/socket.h>
#include <netinet/tcp.h>

#define LIKELY(x) __builtin_expect ((x), 1)
#define UNLIKELY(x) __builtin_expect ((x), 0)

#define PORT 8080
#define BUFFER_SIZE 1024

WasmEdge_VMContext *VMCxt;
WasmEdge_ConfigureContext *ConfCxt;
WasmEdge_Async *Async;
WasmEdge_ModuleInstanceContext *wasi_module;
const char *function_name;

double netns_elapsed, webserver_elapsed, loadwasm_elapsed, execwasm_elapsed, runw_setup;
int execwasm_msec, runw_setup_msec;

struct timespec start, end;
struct timespec setup_start, setup_end;

int WasmVMInit(uint32_t argn, uint32_t envn, const char *dirs[2]) {
    clock_gettime(CLOCK_MONOTONIC, &start);
    // Create new WASM VM context
    VMCxt = WasmEdge_VMCreate(ConfCxt, NULL);
    if (UNLIKELY(VMCxt == NULL)) {
      WasmEdge_ConfigureDelete(ConfCxt);
      error (EXIT_FAILURE, 0, "could not create wasmedge vm");
      return -1;
    }
    wasi_module = WasmEdge_VMGetImportModuleContext (VMCxt, WasmEdge_HostRegistration_Wasi);
    if (UNLIKELY (wasi_module == NULL)) {
      WasmEdge_VMDelete(VMCxt);
      WasmEdge_ConfigureDelete(ConfCxt);
      error (EXIT_FAILURE, 0, "could not get wasmedge wasi module context");
      return -1;
    }
    WasmEdge_ModuleInstanceInitWASI(wasi_module, NULL, argn, NULL, envn, dirs, 1);
    clock_gettime(CLOCK_MONOTONIC, &end);
    loadwasm_elapsed = (end.tv_sec - start.tv_sec) * 1e6 + (end.tv_nsec - start.tv_nsec) / 1e3; // in microseconds
    return 0;
}

int main(int argc, const char* argv[]) {
  clock_gettime(CLOCK_MONOTONIC, &setup_start);
  uint32_t argn = 0;
  uint32_t envn = 0;

  extern char **environ;

  function_name = argv[1];
  const char *dirs[2] = { argv[2] };
  int ns_num = atoi(argv[3]);

  /* Begin join existing network namespace */
  // clock_gettime(CLOCK_PROCESS_CPUTIME_ID, &start);
  clock_gettime(CLOCK_MONOTONIC, &start);
  char path[PATH_MAX];
  snprintf(path, sizeof(path), "/var/run/netns/fe_wasm_netns%d", ns_num);
  int net_ns = open(path, O_RDONLY);
  int status = setns(net_ns, 0);
  if(status != 0) {
    printf("ERROR joining NET NS /var/run/netns/fe_wasm_netns%d\n", ns_num);
    exit(-1);
  }
  else {
    close(net_ns);
  }
  clock_gettime(CLOCK_MONOTONIC, &end);
  netns_elapsed = (end.tv_sec - start.tv_sec) * 1e6 + (end.tv_nsec - start.tv_nsec) / 1e3; // in microseconds
  /* End join existing network namespace */

  /* Begin load WASM deps */
  #ifdef WITH_PLUGINS
  WasmEdge_PluginLoadWithDefaultPaths();
  #endif
  ConfCxt = WasmEdge_ConfigureCreate();
  if (UNLIKELY (ConfCxt == NULL)) {
    error (EXIT_FAILURE, 0, "could not create wasmedge configure");
  }

  WasmEdge_ConfigureAddHostRegistration(ConfCxt, WasmEdge_HostRegistration_Wasi);
  WasmEdge_String fname = WasmEdge_StringCreateByCString("_start");
  /* End load WASM deps */

  /* Begin create web server */
  clock_gettime(CLOCK_MONOTONIC, &start);
  char buffer[BUFFER_SIZE];
  int sockfd = socket(AF_INET, SOCK_STREAM, 0);
  if (sockfd == -1) {
      perror("Unable to create socket for web server");
      return 1;
  }
  if (setsockopt(sockfd, SOL_SOCKET, SO_REUSEADDR, &(int){1}, sizeof(int)) < 0) {
        perror("setsockopt(SO_REUSEADDR) failed");
        return 1;
  }
  if (setsockopt(sockfd, SOL_TCP, TCP_NODELAY, &(int){1}, sizeof(int)) < 0) {
        perror("setsockopt(SO_REUSEADDR) failed");
        return 1;
  }
  struct sockaddr_in host_addr;
  int host_addrlen = sizeof(host_addr);

  host_addr.sin_family = AF_INET;
  host_addr.sin_port = htons(PORT);
  host_addr.sin_addr.s_addr = htonl(INADDR_ANY);

  // Create client address
  struct sockaddr_in client_addr;
  int client_addrlen = sizeof(client_addr);

  if (bind(sockfd, (struct sockaddr *)&host_addr, host_addrlen) != 0) {
      perror("Unable to bind webserver to port");
      return 1;
  }

  if (listen(sockfd, SOMAXCONN) != 0) {
      perror("Webserver unable to listen on port");
      return 1;
  }
  clock_gettime(CLOCK_MONOTONIC, &end);
  webserver_elapsed = (end.tv_sec - start.tv_sec) * 1e6 + (end.tv_nsec - start.tv_nsec) / 1e3; // in microseconds
  /* End create webserver */

  /* Create initial WASM VM */
  WasmVMInit(argn, envn, dirs);

  clock_gettime(CLOCK_MONOTONIC, &setup_end);
  runw_setup = (setup_end.tv_sec - setup_start.tv_sec) * 1e6 + (setup_end.tv_nsec - setup_start.tv_nsec) / 1e3; // in microseconds
  runw_setup /= 1000.0;
  runw_setup_msec = (int)runw_setup;
  if(runw_setup_msec < 1) {
    runw_setup_msec = 1;
  }

  /* Begin serve HTTP reqs */
  while(1) {
    // Accept incoming connections
    int newsockfd = accept(sockfd, (struct sockaddr *)&host_addr,
                            (socklen_t *)&host_addrlen);
    if (newsockfd < 0) {
        perror("webserver (accept)");
        continue;
    }
    // Get client address
    int sockn = getsockname(newsockfd, (struct sockaddr *)&client_addr,
                            (socklen_t *)&client_addrlen);
    if (sockn < 0) {
        perror("webserver (getsockname)");
        continue;
    }

    /* TODO: Implement recv() to read client input from socket 
     * For testing, we just invoke functions with hardcoded vals */
    
    /* Begin exec WASM */
    clock_gettime(CLOCK_MONOTONIC, &start);
    WasmEdge_Result Res = WasmEdge_VMRunWasmFromFile(VMCxt, argv[1], fname, NULL, 0, NULL, 0);
    clock_gettime(CLOCK_MONOTONIC, &end);
    execwasm_elapsed = (end.tv_sec - start.tv_sec) * 1e6 + (end.tv_nsec - start.tv_nsec) / 1e3; // in microseconds
    execwasm_elapsed /= 1000.0;
    execwasm_msec = (int)execwasm_elapsed;

    /* Clean up existing WASM VM */
    double wasm_cleanup;
    clock_gettime(CLOCK_MONOTONIC, &start);
    WasmEdge_VMDelete(VMCxt);
    clock_gettime(CLOCK_MONOTONIC, &end);
    wasm_cleanup = (end.tv_sec - start.tv_sec) * 1e6 + (end.tv_nsec - start.tv_nsec) / 1e3; // in microseconds

    char *resp;
    int rsz;
    if (WasmEdge_ResultOK(Res)) {
      rsz = snprintf(NULL, 0, "HTTP/1.1 200 OK\r\nServer: runw\r\nContent-type: text/plain\r\nInvocation-elapsed: %d\r\nrunw-setup: %d\r\nMisc-stats: %f,%f,%f,%f\r\n\r\nSuccess\r\n", execwasm_msec, runw_setup_msec, netns_elapsed, webserver_elapsed, loadwasm_elapsed, wasm_cleanup);
      resp = malloc(rsz + 1);
      sprintf(resp, "HTTP/1.1 200 OK\r\nServer: runw\r\nContent-type: text/plain\r\nInvocation-elapsed: %d\r\nrunw-setup: %d\r\nMisc-stats: %f,%f,%f,%f\r\n\r\nSuccess\r\n", execwasm_msec, runw_setup_msec, netns_elapsed, webserver_elapsed, loadwasm_elapsed, wasm_cleanup);
    } else {
      rsz = snprintf(NULL, 0, "HTTP/1.1 200 OK\r\nServer: runw\r\nContent-type: text/plain\r\n\r\nFailure\r\n");
      resp = malloc(rsz+1);
      sprintf(resp, "HTTP/1.1 200 OK\r\nServer: runw\r\nContent-type: text/plain\r\n\r\nFailure\r\n");
    }

    if (send(newsockfd, resp, rsz, 0) == -1) {
      perror("webserver (send)");
    }

    free(resp);
    close(newsockfd);
    fprintf(stderr, "%d,%d,%f,%f,%f,%f\n------------\n", execwasm_msec, runw_setup_msec, netns_elapsed, webserver_elapsed, loadwasm_elapsed, wasm_cleanup);
    /* Create new WASM VM */
    WasmVMInit(argn, envn, dirs);
  }

  close(sockfd);
/*
  WasmEdge_StringDelete(WasmEdge_StringCreateByCString("_start"));
  WasmEdge_VMDelete(VMCxt);
  WasmEdge_ConfigureDelete(ConfCxt);
*/
  return 0;
}

package handlers

import (
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

func Reserve(ip string, id string, fs *FunctionStore) string {
	log.Printf("[Reserve] Asked to reserve ip=%s for ctr=%s\n", ip, id)
	start := time.Now()
	//if _, ok := fs.ipconfigs[id]; !ok {
	if _, ok := fs.ipconfigs.Load(id); !ok {
		// fs.ipconfigs[id] = ip
		fs.ipconfigs.Store(id, ip)
		duration := time.Since(start)
		log.Printf("[ipam.Reserve] Took %d ms", duration.Milliseconds())
	} else {
		/* If reservation for IP already exists, return error */
		duration := time.Since(start)
		log.Printf("[Reserve] Container '%s' already has IP reserved\n", id)
		log.Printf("[ipam.Reserve] Took %d ms", duration.Milliseconds())
		return "false"
	}
	return "true"
}

func LastReservedIP(fs *FunctionStore) string {
	start := time.Now()
	fs.metricMu.Lock()
	lastUsed := fs.nextIP
	fs.nextIP = getNextIP(lastUsed, 1)
	fs.metricMu.Unlock()
	log.Printf("[LastReservedIP] Returning lastUsed=%s\n", lastUsed)
	duration := time.Since(start)
	log.Printf("[ipam.LastReservedIP] Took %d ms", duration.Milliseconds())
	return lastUsed.String()
}

func getNextIP(ip net.IP, inc uint) net.IP {
	i := ip.To4()
	v := uint(i[0])<<24 + uint(i[1])<<16 + uint(i[2])<<8 + uint(i[3])
	v += inc
	v3 := byte(v & 0xFF)
	v2 := byte((v >> 8) & 0xFF)
	v1 := byte((v >> 16) & 0xFF)
	v0 := byte((v >> 24) & 0xFF)
	return net.IPv4(v0, v1, v2, v3)
}

func FindBy(id string, fs *FunctionStore) string {
	log.Printf("[FindBy] Looking up reservation for '%s'\n", id)
	start := time.Now()
	// if _, ok := fs.ipconfigs[id]; !ok {
	if _, ok := fs.ipconfigs.Load(id); !ok {
		log.Printf("[FindBy] Reservation NOT found for '%s'\n", id)
		duration := time.Since(start)
		log.Printf("[ipam.FindBy] Took %d ms", duration.Milliseconds())
		return "false"
	}
	log.Printf("[FindBy] Reservation found for '%s'\n", id)
	duration := time.Since(start)
	log.Printf("[ipam.FindBy] Took %d ms", duration.Milliseconds())
	return "true"
}

func ReleaseBy(id string, fs *FunctionStore) string {
	log.Printf("[ReleaseBy] Releasing reservation for '%s'\n", id)
	start := time.Now()
	// if _, ok := fs.ipconfigs[id]; !ok {
	if _, ok := fs.ipconfigs.Load(id); !ok {
		log.Printf("[ReleaseBy] Reservation NOT found for '%s'\n", id)
		duration := time.Since(start)
		log.Printf("[ipam.ReleaseBy] Took %d ms", duration.Milliseconds())
		return "false"
	}
	// delete(fs.ipconfigs, id)
	fs.ipconfigs.Delete(id)
	duration := time.Since(start)
	log.Printf("[ipam.ReleaseBy] Took %d ms", duration.Milliseconds())
	return "true"
}

func (fs *FunctionStore) GetBy(id string) string {
	// if _, ok := fs.ipconfigs[id]; ok {
	start := time.Now()
	if reservedIP, ok := fs.ipconfigs.Load(id); ok {
		log.Printf("[GetBy] Returning reservation ip=%s for ctr='%s'\n", reservedIP, id)
		duration := time.Since(start)
		log.Printf("[ipam.GetBy] Took %d ms", duration.Milliseconds())
		return reservedIP.(string)
	} else {
		/* If reservation doesn't exist, return error */
		log.Printf("[GetBy] Reservation not found for '%s'\n", id)
		duration := time.Since(start)
		log.Printf("[ipam.GetBy] Took %d ms", duration.Milliseconds())
		return ""
	}
}

// MakeIPAMHandler saves container IP configs and tracks next available IP
func MakeIPAMHandler(fs *FunctionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}
		id := strings.TrimSpace(r.Header.Get("id"))
		retval := "false"
		switch action := r.Header.Get("action"); action {
		case "Reserve":
			ip := r.Header.Get("ip")
			retval = Reserve(ip, id, fs)
		case "LastReservedIP":
			retval = LastReservedIP(fs)
		case "FindByKey":
			retval = FindBy(id, fs)
		case "FindByID":
			retval = FindBy(id, fs)
		case "ReleaseByKey":
			retval = ReleaseBy(id, fs)
		case "ReleaseByID":
			retval = ReleaseBy(id, fs)
		case "GetByID":
			retval = fs.GetBy(id)
		}

		// jsonOut, marshalErr := json.Marshal(lastUsed.String())
		// if marshalErr != nil {
		// 	w.WriteHeader(http.StatusInternalServerError)
		// 	return
		// }

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(retval))
	}
}

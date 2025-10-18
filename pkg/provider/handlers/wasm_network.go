package handlers

import (
	"fmt"
	"log"
	"os/exec"

	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

func (fs *FunctionStore) AddWasmInterface(thisNS string, thisIF string, thisIP string, thisCIDR string) error {
	/* Add network namespace */
	cmd := exec.Command("ip", "netns", "add", thisNS)
	s, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to create network namespace for netns '%s': %s", thisNS, s)
		return err
	}

	/* Add veth interface */
	fmt.Println("Adding veth interface")
	cmd = exec.Command("ip", "link", "add", "eth1", "type", "veth", "peer", "name", thisIF)
	s, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to create veth netns '%s': %s", thisNS, s)
		return err
	}

	/* Add veth interface to new netns */
	fmt.Println("Adding veth interface to new netns")
	cmd = exec.Command("ip", "link", "set", "eth1", "netns", thisNS)
	s, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to add veth to netns '%s': %s", thisNS, s)
		return err
	}

	/* Assign IP to new veth */
	fmt.Println("Adding IP to new veth")
	cmd = exec.Command("ip", "-n", thisNS, "addr", "add", thisCIDR, "dev", "eth1")
	s, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to add IP to veth in netns '%s': %s", thisNS, s)
		return err
	}

	/* Bring up veth inside netns */
	fmt.Println("Bring up veth inside netns")
	cmd = exec.Command("ip", "-n", thisNS, "link", "set", "eth1", "up")
	s, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to bring up veth inside netns '%s': %s", thisNS, s)
		return err
	}

	/* Bring up loopback */
	fmt.Println("Bring up loopback inside netns")
	cmd = exec.Command("ip", "-n", thisNS, "link", "set", "lo", "up")
	s, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to bring up loopback inside netns '%s': %s", thisNS, s)
		return err
	}

	/* Bring up veth on host */
	fmt.Println("Bring up veth on host")
	cmd = exec.Command("ip", "link", "set", thisIF, "up")
	s, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to bring up veth on host for netns '%s': %s", thisNS, s)
		return err
	}

	/* Tie veth on host to bridge */
	fmt.Println("Tie veth on host to bridge")
	cmd = exec.Command("ip", "link", "set", thisIF, "master", "fewasm0")
	s, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to tie veth on host to bridge for netns '%s': %s", thisNS, s)
		return err
	}

	/* Add default route to bridge inside netns */
	fmt.Println("Add default route to bridge inside netns")
	cmd = exec.Command("ip", "netns", "exec", thisNS, "ip", "route", "add", "default", "via", "10.63.0.1")
	s, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("[AddWasmInterface] Failed to add default route in netns '%s': %s", thisNS, s)
		return err
	}
	return nil
}

func (fs *FunctionStore) CreateWasmInterfaces(numNS int) error {
	fs.nsMu.Lock()
	defer fs.nsMu.Unlock()
	if numNS > 1000 {
		numNS = 1000
	}
	v3 := 1
	v2 := 100
	v1 := 63
	v0 := 10
	var err error
	for ns := 1; ns <= numNS; ns++ {
		if v3 > 254 {
			v3 = 1
			v2 += 1
		}
		thisIP := fmt.Sprintf(`%d.%d.%d.%d`, v0, v1, v2, v3)
		ipInfo := wasmIPInfo{}
		ipInfo.netnsNum = ns
		ipInfo.IP = thisIP

		fs.netnsList = append(fs.netnsList, ipInfo)
		v3 += 1
	}
	timec.LogEvent("function_store/CreateWasmInterfaces", fmt.Sprintf("Created %d WASM interfaces. First interface: netnsNum=%d, IP=%s", numNS, fs.netnsList[0].netnsNum, fs.netnsList[0].IP), 2)
	return err
}

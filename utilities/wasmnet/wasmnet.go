package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	gocni "github.com/containerd/go-cni"
)

func dirExists(dirname string) bool {
	exists, info := pathExists(dirname)
	if !exists {
		return false
	}

	return info.IsDir()
}

func pathExists(path string) (bool, os.FileInfo) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}

	return true, info
}

func createBridge() error {
	const (
		// CNIBinDir describes the directory where the CNI binaries are stored
		CNIBinDir = "/opt/cni/bin"
		// CNIConfDir describes the directory where the CNI plugin's configuration is stored
		CNIConfDir = "/etc/cni/net.d"
		// CNIDataDir is the directory CNI stores allocated IP for containers
		CNIDataDir          = "/var/run/cni"
		wasmCNIConfFilename = "10-fe-wasm.conflist"
		wasmNetworkName     = "fe-wasm-cni-bridge"
		wasmBridgeName      = "fewasm0"
		wasmSubnet          = "10.63.0.0/16"
		defaultIfPrefix     = "eth"
	)
	var wasmCNIConf = fmt.Sprintf(`
    {
        "cniVersion": "0.4.0",
        "name": "%s",
        "plugins": [
        {
            "type": "bridge",
            "bridge": "%s",
            "isGateway": true,
            "ipMasq": false,
            "ipam": {
                "type": "host-local",
                "subnet": "%s",
                "dataDir": "%s",
                "routes": [
                    { "dst": "0.0.0.0/0" }
                ]
            }
        }
        ]
    }
    `, wasmNetworkName, wasmBridgeName, wasmSubnet, CNIDataDir)

	log.Printf("Checking for config dir and creating if needed...\n")
	if !dirExists(CNIConfDir) {
		if err := os.MkdirAll(CNIConfDir, 0755); err != nil {
			return fmt.Errorf("Cannot create CNI conf directory: %s", CNIConfDir)
		}
	}

	log.Printf("Writing WASM network config...\n")
	netConfigWasm := path.Join(CNIConfDir, wasmCNIConfFilename)
	if err := ioutil.WriteFile(netConfigWasm, []byte(wasmCNIConf), 644); err != nil {
		return fmt.Errorf("Cannot write WASM network config: %s", wasmCNIConfFilename)
	}

	log.Printf("Initializing CNI\n")
	cni, err := gocni.New(
		gocni.WithPluginConfDir(CNIConfDir),
		gocni.WithPluginDir([]string{CNIBinDir}),
		gocni.WithInterfacePrefix(defaultIfPrefix),
	)

	if err != nil {
		return fmt.Errorf("error initializing cni: %s", err)
	}

	// Load the cni WASM configuration
	log.Printf("Loading CNI WASM configuration\n")
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithConfListFile(filepath.Join(CNIConfDir, wasmCNIConfFilename))); err != nil {
		return fmt.Errorf("failed to load cni WASM configuration: %v", err)
	}

	return nil
}

func main() {

	log.Printf("Not creating bridge (this should have been created manually during firewall/interface setup)\n")

	var numNS int64 = 500
	v3 := 1
	v2 := 100
	v1 := 63
	v0 := 10
	var ns int64
	for ns = 1; ns <= numNS; ns++ {
		if v3 > 254 {
			v3 = 1
			v2 += 1
		}
		thisIP := fmt.Sprintf(`%d.%d.%d.%d`, v0, v1, v2, v3)
		thisCIDR := thisIP + "/16"
		thisNS := fmt.Sprintf(`fe_wasm_netns%d`, ns)
		thisIF := fmt.Sprintf(`fe_wasm_eth%d`, ns)
		fmt.Printf("Adding %s to ns %s\n", thisIP, thisNS)
		/* Add network namespace */
		cmd := exec.Command("ip", "netns", "add", thisNS)
		s, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error adding new netns: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		/* Add veth interface */
		cmd = exec.Command("ip", "link", "add", "eth1", "type", "veth", "peer", "name", thisIF)
		s, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error creating eth1: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		/* Add veth interface to new netns */
		cmd = exec.Command("ip", "link", "set", "eth1", "netns", thisNS)
		s, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error adding eth to netns: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		/* Assign IP to new veth */
		cmd = exec.Command("ip", "-n", thisNS, "addr", "add", thisCIDR, "dev", "eth1")
		s, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error adding addr to eth1: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		/* Bring up veth inside netns */
		cmd = exec.Command("ip", "-n", thisNS, "link", "set", "eth1", "up")
		s, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error setting eth1 to up: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		/* Bring up loopback */
		cmd = exec.Command("ip", "-n", thisNS, "link", "set", "lo", "up")
		s, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error setting loopback to up: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		/* Bring up veth on host */
		cmd = exec.Command("ip", "link", "set", thisIF, "up")
		s, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error bringing up veth on host: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		/* Tie veth on host to bridge */
		cmd = exec.Command("ip", "link", "set", thisIF, "master", "fewasm0")
		s, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error tying veth to bridge: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		/* Add default route to bridge inside netns */
		cmd = exec.Command("ip", "netns", "exec", thisNS, "ip", "route", "add", "default", "via", "10.63.0.1")
		s, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error setting default route inside netns: %s (%s)\n", s, err)
			log.Fatal(err)
		}

		v3 += 1
	}
}

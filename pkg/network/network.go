package network

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"time"

	"github.com/urfave/cli"
	"weike.sh/mydocker/util"
)

func NewNetwork(ctx *cli.Context) (*Network, error) {
	if len(ctx.Args()) < 1 {
		return nil, fmt.Errorf("missing network's name")
	}

	name := ctx.Args().Get(0)
	for nwName := range Networks {
		if nwName == name {
			return nil, fmt.Errorf("the network name %s already exists", name)
		}
	}

	driver := ctx.String("driver")
	if driver == "" {
		return nil, fmt.Errorf("missing --driver option")
	}

	subnet := ctx.String("subnet")
	if subnet == "" {
		return nil, fmt.Errorf("missing --subnet option")
	}

	// e.g. parse "10.20.30.1/24" to "10.20.30.0/24"
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, err
	}

	// set the gateway ip as the first ip addr of the subnet.
	// e.g. set gateway to 10.20.30.1 for subnet 10.20.30.0/24
	gateway := GetIPFromSubnetByIndex(ipNet, 1)

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if addr.String() == gateway.String() {
			return nil, fmt.Errorf("the subnet %s already exists", ipNet)
		}
	}

	nw := &Network{
		Name:       name,
		Counts:     0,
		Driver:     driver,
		IPNet:      ipNet,
		Gateway:    gateway,
		CreateTime: time.Now().Format("2006-01-02 15:04:05"),
	}

	Networks[name] = nw
	return nw, nil
}

func (nw *Network) ConfigFileName() (string, error) {
	configDir := path.Join(DriversDir, nw.Driver)
	configFileName := path.Join(configDir, nw.Name+".json")
	if err := util.EnSureFileExists(configFileName); err != nil {
		return "", err
	}
	return configFileName, nil
}

func (nw *Network) Create() error {
	if err := Drivers[nw.Driver].Create(nw); err != nil {
		return err
	}
	if err := IPAllocator.Init(nw); err != nil {
		return err
	}
	return nw.Dump()
}

func (nw *Network) Delete() error {
	if nw.Counts > 0 {
		return fmt.Errorf("there still exist %d ips in subnet %s",
			nw.Counts, nw.IPNet)
	} else {
		if err := IPAllocator.Init(nw); err != nil {
			return err
		}
		delete(*IPAllocator.SubnetBitMap, nw.IPNet.String())
		if err := IPAllocator.Dump(); err != nil {
			return err
		}
	}

	if err := Drivers[nw.Driver].Delete(nw); err != nil {
		return err
	}

	if configFileName, err := nw.ConfigFileName(); err == nil {
		return os.Remove(configFileName)
	} else {
		return err
	}
}

func (nw *Network) Dump() error {
	configFileName, err := nw.ConfigFileName()
	if err != nil {
		return err
	}

	jsonBytes, err := json.Marshal(nw)
	if err != nil {
		return fmt.Errorf("failed to json-encode network %s: %v",
			nw.Name, err)
	}

	// WriteFile will create the file if it doesn't exist,
	// otherwise WriteFile will truncate it before writing
	if err := ioutil.WriteFile(configFileName, jsonBytes, 0644); err != nil {
		return fmt.Errorf("failed to write network config to file %s: %v",
			configFileName, err)
	}

	return nil
}

func (nw *Network) Load() error {
	configFileName, err := nw.ConfigFileName()
	if err != nil {
		return err
	}

	jsonBytes, err := ioutil.ReadFile(configFileName)
	if len(jsonBytes) == 0 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read configFile %s: %v",
			configFileName, err)
	}

	if err := json.Unmarshal(jsonBytes, nw); err != nil {
		return fmt.Errorf("failed to json-decode network %s: %v",
			nw.Name, err)
	}

	return nil
}

func (nw *Network) MarshalJSON() ([]byte, error) {
	type nwAlias Network
	return json.Marshal(&struct {
		IPNet   string `json:"IPNet"`
		Gateway string `json:"Gateway"`
		*nwAlias
	}{
		IPNet:   nw.IPNet.String(),
		Gateway: nw.Gateway.IP.String(),
		nwAlias: (*nwAlias)(nw),
	})
}

func (nw *Network) UnmarshalJSON(data []byte) error {
	type nwAlias Network
	aux := &struct {
		IPNet   string `json:"IPNet"`
		Gateway string `json:"Gateway"`
		*nwAlias
	}{
		nwAlias: (*nwAlias)(nw),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	_, ipNet, err := net.ParseCIDR(aux.IPNet)
	if err != nil {
		return err
	}

	nw.IPNet = ipNet
	nw.Gateway = GetIPFromSubnetByIndex(ipNet, 1)

	return nil
}

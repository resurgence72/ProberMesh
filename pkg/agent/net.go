package agent

import (
	"context"
	"net"
	"os"
	"os/exec"
	"time"
)

const (
	defaultRegionEnv = "PROBER_REGION"
	defaultRegion    = "cn-shanghai"
	defaultRegionCmd = "curl -s http://100.100.100.200/latest/meta-data/region-id"
)

func getLocalIP() string {
	var (
		addrs   []net.Addr
		addr    net.Addr
		ipNet   *net.IPNet
		isIpNet bool
		err     error
	)

	// 获取本机网卡的ip
	if addrs, err = net.InterfaceAddrs(); err != nil {
		return "0.0.0.0"
	}
	// 取第一个非lo的网卡IP
	for _, addr = range addrs {
		// ipv4  ipv6
		if ipNet, isIpNet = addr.(*net.IPNet); isIpNet && !ipNet.IP.IsLoopback() {
			// 跳过ipv6
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "0.0.0.0"
}

func getSelfRegion(dr string) string {
	// 1. 如果flag指定了region,使用指定
	if len(dr) > 0 {
		return dr
	}

	// 2. 未指定region, 获取env
	if region, ok := os.LookupEnv(defaultRegionEnv); ok && len(region) > 0 {
		return region
	}

	// 3. env没有 使用curl
	ctx, _ := context.WithTimeout(context.TODO(), time.Duration(2)*time.Second)
	cmd := exec.CommandContext(
		ctx,
		"bash",
		"-c",
		defaultRegionCmd,
	)

	bs, err := cmd.CombinedOutput()
	if err != nil {
		// 4. curl不到，使用默认 cn-shanghai
		return defaultRegion
	}
	return string(bs)
}

package agent

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultRegionEnv = "PROBER_REGION"
	defaultRegion    = "cn-shanghai"
	regionTimeout    = time.Duration(3) * time.Second
)

const (
	intranetNetType = "intranet"
	publicNetType   = "public"
)

var agentIP string

func initAgentLocalIP(networkType string) {
	if networkType == intranetNetType {
		agentIP = getIntranetIP()
	}
	agentIP = getPublicIP()
}

func getIntranetIP() string {
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

func getPublicIP() string {
	ctx, _ := context.WithTimeout(context.TODO(), regionTimeout)
	cmd := exec.CommandContext(
		ctx,
		"bash",
		"-c",
		"curl -s ifconfig.me",
	)
	bs, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Errorln("agent get public ip failed", err)
		return getIntranetIP()
	}
	return string(bs)
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
	var (
		aliCloudRegion    = "http://100.100.100.200/latest/meta-data/region-id"
		tencentRegion     = "http://metadata.tencentyun.com/latest/meta-data/placement/zone"
		googleCloudRegion = "http://metadata.google.internal/computeMetadata/v1/instance/zone"
	)

	f := func(r string) (string, error) {
		ctx, _ := context.WithTimeout(context.TODO(), regionTimeout)
		cmd := exec.CommandContext(
			ctx,
			"bash",
			"-c",
			fmt.Sprintf("curl -s %s", r),
		)

		bs, err := cmd.CombinedOutput()
		return string(bs), err
	}

	pipeLines := []func() (string, error){
		func() (string, error) {
			return f(aliCloudRegion)
		},
		func() (string, error) {
			return f(tencentRegion)
		},
		func() (string, error) {
			region, err := f(googleCloudRegion)
			if err == nil && len(region) > 0 {
				ss := strings.Split(region, "/")
				region = ss[len(ss)-1]
			}
			return region, err
		},
	}

	for _, fn := range pipeLines {
		if region, err := fn(); err == nil {
			return region
		}
	}

	// 4. curl不到，使用默认 cn-shanghai
	return defaultRegion
}

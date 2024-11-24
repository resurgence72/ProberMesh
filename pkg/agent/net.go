package agent

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const (
	defaultRegionEnv = "PROBER_REGION"
	regionTimeout    = time.Second
)

const (
	intranetNetType = "intranet"
	publicNetType   = "public"
)

var (
	agentIP string
	region  = "cn-shanghai"
)

func initAgentLocalIP(networkType string) {
	if networkType == intranetNetType {
		agentIP = getIntranetIP()
		return
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
		"/usr/bin/curl -s ifconfig.me",
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
	if r, ok := os.LookupEnv(defaultRegionEnv); ok && len(r) > 0 {
		region = r
		return r
	}

	// 3. env没有 使用curl
	var (
		getAliCloudRegionCmd    = `curl -s http://100.100.100.200/latest/meta-data/region-id`
		getTencentRegionCmd     = `curl -s http://metadata.tencentyun.com/latest/meta-data/placement/zone`
		getGoogleCloudRegionCmd = `curl -s http://metadata.google.internal/computeMetadata/v1/instance/zone -H "Metadata-Flavor: Google"`
		getHuaWeiCloudRegionCmd = `curl -s http://169.254.169.254/openstack/latest/meta_data.json`
		getVolcEngineRegionCmd  = `curl -s http://100.96.0.96/latest/region_id`
		getAWSRegionCmd         = `curl -s http://169.254.169.254/latest/meta-data/placement/region`
	)

	ctx, cancel := context.WithTimeout(context.Background(), regionTimeout)
	defer cancel()

	resultCn := make(chan string, 1)
	f := func(r string) (string, error) {
		cmd := exec.CommandContext(
			ctx,
			"bash",
			"-c",
			r,
		)

		bs, err := cmd.CombinedOutput()
		return string(bs), err
	}

	pipeLines := []func(){
		func() {
			r, err := f(getAliCloudRegionCmd)
			if err == nil && len(r) > 0 {
				resultCn <- r
			}
		},
		func() {
			r, err := f(getTencentRegionCmd)
			if err == nil && len(r) > 0 {
				resultCn <- r
			}
		},
		func() {
			r, err := f(getGoogleCloudRegionCmd)
			if err == nil && len(r) > 0 {
				ss := strings.Split(r, "/")
				r = ss[len(ss)-1]
				resultCn <- r
			}
		},
		func() {
			r, err := f(getHuaWeiCloudRegionCmd)
			if err == nil && len(r) > 0 {
				parser := gjson.Parse(r)
				if parser.IsObject() && parser.Get("availability_zone").Exists() {
					resultCn <- parser.Get("availability_zone").String()
				}
			}
		},
		func() {
			r, err := f(getVolcEngineRegionCmd)
			if err == nil && len(r) > 0 {
				resultCn <- r
			}
		},
		func() {
			r, err := f(getAWSRegionCmd)
			if err == nil && len(r) > 0 {
				resultCn <- r
			}
		},
	}

	for i := range pipeLines {
		go pipeLines[i]()
	}

	select {
	case region = <-resultCn:
	case <-time.After(3 * regionTimeout):
		logrus.Warnln("get region timeout, use default region: [cn-shanghai]")
	}

	close(resultCn)
	return region
}

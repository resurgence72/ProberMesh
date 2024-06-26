package pb

import (
	"regexp"

	"probermesh/pkg/config"
)

type ReportReq struct {
	IP      string `json:"ip"`
	Region  string `json:"region"`
	Version string `json:"version"`
}

type TargetPoolReq struct {
	SourceRegion string
}

type TargetPoolResp struct {
	Targets map[string][]*config.ProberConfig
}

type ProberResultReq struct {
	// 探测类型
	ProberType string

	// 探测地址
	ProberTarget string

	// 本地ip
	LocalIP string

	// 原region
	SourceRegion string

	// 目的region
	TargetRegion string

	// 探测是否成功
	ProberSuccess bool

	// http探测失败原因
	ProberFailedReason string
	// http响应码
	ProberStatusCode int
	// http字段
	HTTPFields map[string]float64
	// tls字段
	TLSFields map[string]string

	// icmp的字段 resolve setup rtt
	ICMPFields map[string]float64
}

type UpgradeCheckReq struct {
	Version string
	Ident   string
}

type UpgradeResp struct {
	Upgraded    bool   `json:"upgraded"`
	Md5Check    string `json:"md5Check"`
	DownloadURL string `json:"downloadURL"`
}

type TaskReq struct {
	Region    string `json:"region"`
	RegionReg *regexp.Regexp

	Expr string `json:"expr"`
	Cmd  string `json:"cmd"`
}

type ReportTaskResultReq struct {
	Ident  string
	Cmd    string
	Result string
}

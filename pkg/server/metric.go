package server

import "github.com/prometheus/client_golang/prometheus"

var (
	namespace = "prober"

	// server 接收到的上报数量
	serverReceivePointsVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "server_receive_points",
		Help:      "number of reported points received by the server",
	}, []string{"prober_type"})

	// icmp 探测失败数量
	icmpProberFailedGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "icmp_failed",
		Help:      "icmp prober failed times",
	}, []string{"source_region", "target_region"})

	// icmp 分阶段耗时
	icmpProberDurationGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "icmp_duration_seconds",
		Help:      "icmp prober duration by phase",
	}, []string{"phase", "source_region", "target_region"})

	// icmp 丢包率
	icmpProberPacketLossRateGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "icmp_packet_loss_rate",
		Help:      "icmp prober packet loss rate",
	}, []string{"source_region", "target_region"})

	// icmp 时延标准差 stddev
	icmpProberJitterStdDevGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "icmp_jitter_stddev_seconds",
		Help:      "icmp prober network jitter std dev seconds",
	}, []string{"source_region", "target_region"})

	// icmp 总耗时分布
	icmpProberDurationHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "icmp_duration_seconds_total",
		Help:      "icmp prober duration histogram by phase",
		// 5e-05,0.0001,0.0002,0.0004,0.0008,0.0016,0.0032,0.0064,0.0128,0.0256,0.0512,0.1024,0.2048,0.4096,0.8192,1.6384,3.2768,6.5536,13.1072
		Buckets: prometheus.ExponentialBuckets(0.00005, 2, 19),
	}, []string{"source_region", "target_region"})

	// http 探测失败数量
	httpProberFailedGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "http_failed",
		Help:      "http prober failed times",
	}, []string{"source_region", "target_addr", "failed_reason"})

	// http 分阶段耗时
	httpProberDurationGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "http_duration_seconds",
		Help:      "http prober duration by phase",
	}, []string{"phase", "source_region", "target_addr"})

	// agent 节点健康检查
	agentHealthCheckGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "agent_is_alive",
		Help:      "proberMesh agent is alive",
	}, []string{"region", "ip", "version"})
)

func init() {
	prometheus.MustRegister(icmpProberFailedGaugeVec)
	prometheus.MustRegister(icmpProberDurationGaugeVec)
	prometheus.MustRegister(icmpProberDurationHistogramVec)
	prometheus.MustRegister(icmpProberPacketLossRateGaugeVec)
	prometheus.MustRegister(icmpProberJitterStdDevGaugeVec)
	prometheus.MustRegister(httpProberFailedGaugeVec)
	prometheus.MustRegister(httpProberDurationGaugeVec)
	prometheus.MustRegister(serverReceivePointsVec)
}

package server

import "github.com/prometheus/client_golang/prometheus"

var (
	namespace = "prober"

	// server 接收到的上报数量  # 不考虑恢复
	serverReceivePointsVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "server_receive_points",
		Help:      "number of reported points received by the server",
	}, []string{"prober_type"})

	// icmp 探测失败数量 # 考虑恢复; agent下线后失败的series需要删除
	icmpProberFailedGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "icmp_failed",
		Help:      "icmp prober failed times",
	}, []string{"source_region", "target_region"})

	// icmp 分阶段耗时  # 考虑恢复，agent下线后历史series需要删除
	icmpProberDurationGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "icmp_duration_seconds",
		Help:      "icmp prober duration by phase",
	}, []string{"phase", "source_region", "target_region"})

	// icmp 丢包率  # 考虑恢复，agent下线后历史series需要删除
	icmpProberPacketLossRateGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "icmp_packet_loss_rate",
		Help:      "icmp prober packet loss rate",
	}, []string{"source_region", "target_region"})

	// icmp 时延标准差 stddev  # 考虑恢复，agent下线后历史series需要删除
	icmpProberJitterStdDevGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "icmp_jitter_stddev_seconds",
		Help:      "icmp prober network jitter std dev seconds",
	}, []string{"source_region", "target_region"})

	// icmp 总耗时分布 # 考虑恢复，agent下线后历史series需要删除
	icmpProberDurationHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "icmp_duration_seconds_total",
		Help:      "icmp prober duration histogram by phase",
		// 5e-05,0.0001,0.0002,0.0004,0.0008,0.0016,0.0032,0.0064,0.0128,0.0256,0.0512,0.1024,0.2048,0.4096,0.8192,1.6384,3.2768,6.5536,13.1072
		Buckets: prometheus.ExponentialBuckets(0.00005, 2, 19),
	}, []string{"source_region", "target_region"})

	// http 探测失败数量 # 考虑恢复，agent下线后历史series需要删除
	httpProberFailedGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "http_failed",
		Help:      "http prober failed times",
	}, []string{"source_region", "target_region", "target_addr", "failed_reason"})

	// http 分阶段耗时 # 考虑恢复，agent下线后历史series需要删除
	httpProberDurationGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "http_duration_seconds",
		Help:      "http prober duration by phase",
	}, []string{"phase", "source_region", "target_region", "target_addr"})

	// http 证书过期信息 # 考虑恢复，agent下线后历史series需要删除
	httpSSLEarliestCertExpiryGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "http_ssl_earliest_cert_expiry",
		Help:      "https last SSL chain expiry in unixtime",
	}, []string{"source_region", "target_region", "target_addr", "version"})

	// agent 节点健康检查   # 考虑恢复，agent下线后历史series需要重置为0
	agentHealthCheckGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "agent_is_alive",
		Help:      "proberMesh agent is alive",
	}, []string{"region", "ip", "version"})

	proberMeshServerTaskEnabledGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "server_task_enabled",
		Help:      "proberMesh server whether allow task module",
	})

	proberMeshServerProbeSelfEnabledGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "server_probe_self_enabled",
		Help:      "proberMesh server whether allow probe.self module",
	})
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
	prometheus.MustRegister(agentHealthCheckGaugeVec)
	prometheus.MustRegister(httpSSLEarliestCertExpiryGaugeVec)
	prometheus.MustRegister(proberMeshServerTaskEnabledGauge)
	prometheus.MustRegister(proberMeshServerProbeSelfEnabledGauge)
}

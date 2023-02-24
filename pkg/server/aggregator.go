package server

import (
	"context"
	"strconv"
	"sync"
	"time"

	"probermesh/pkg/pb"
	"probermesh/pkg/util"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
)

type Aggregator struct {
	queue           [][]*pb.PorberResultReq
	aggInterval     time.Duration
	httpMetricsHold *cache.Cache // hold 点，过期reason自动删除
	icmpMetricsHold *cache.Cache // 过期icmp region自动删除

	cancel context.Context
	m      sync.Mutex
}

type aggProberResult struct {
	sourceRegion string
	targetRegion string

	targetAddr   string // http 使用
	failedReason string //http 使用
	tlsVersion   string //http 使用
	tlsExpiry    int64  //http 使用

	batchCnt int64 // cnt算avg

	failedCnt int64
	phase     map[string]float64
}

var aggregator *Aggregator

func newAggregator(
	ctx context.Context,
	interval time.Duration,
	ratio int,
) *Aggregator {
	// 构建cache
	// 设置agg频率，防止边界情况下拉取时被删除导致拉取不到正常上报数据
	cacheInterval := time.Duration(ratio) * interval
	hmh := cache.New(cacheInterval, cacheInterval)
	hmh.OnEvicted(func(key string, i interface{}) {
		defer func() {
			_ = recover()
		}()

		ks := util.SplitKey(key)
		// 设置删除回调函数; 当此次agg周期内未上报相同key时，说明当前series已恢复，就不要再暴露这个series了
		httpProberDurationGaugeVec.DeleteLabelValues(ks...)
		httpProberFailedGaugeVec.DeleteLabelValues(ks...)
	})

	imh := cache.New(cacheInterval, cacheInterval)
	imh.OnEvicted(func(key string, i interface{}) {
		defer func() {
			// 由于标签数量不同导致prom-go-sdk报错
			_ = recover()
		}()

		ks := util.SplitKey(key)
		icmpProberFailedGaugeVec.DeleteLabelValues(ks...)
		icmpProberPacketLossRateGaugeVec.DeleteLabelValues(ks...)
		icmpProberJitterStdDevGaugeVec.DeleteLabelValues(ks...)
		icmpProberDurationHistogramVec.DeleteLabelValues(ks...)
		icmpProberDurationGaugeVec.DeleteLabelValues(ks...)
	})

	aggregator = &Aggregator{
		queue:           make([][]*pb.PorberResultReq, 0),
		aggInterval:     interval,
		cancel:          ctx,
		httpMetricsHold: hmh,
		icmpMetricsHold: imh,
	}
	return aggregator
}

func (a *Aggregator) Enqueue(reqs []*pb.PorberResultReq) {
	a.m.Lock()
	defer a.m.Unlock()
	a.queue = append(a.queue, reqs)
}

func (a *Aggregator) startAggregation() {
	util.Wait(a.cancel, a.aggInterval, a.agg)
}

func (a *Aggregator) agg() {
	if len(a.queue) == 0 {
		logrus.Warnln("current batch no agent report, continue...")
		return
	}
	logrus.Warnln("has batch report to agg ", len(a.queue))

	var (
		/*
			icmpAggMap = {
			"beijing->shanghai": []PorberResultReq
			"shanghai->beijing": []PorberResultReq
			}
		*/
		icmpAggMap = make(map[string]*aggProberResult)
		httpAggMap = make(map[string]*aggProberResult)

		httpDefaultFailedReason = "success"
	)

	a.m.Lock()
	defer func() {
		a.m.Unlock()
		a.reset()
	}()

	for _, prs := range a.queue {
		for _, pr := range prs {
			var (
				containers map[string]*aggProberResult
				phase      map[string]float64
				key        string

				pt = pr.ProberType
			)

			// 打点上报数量counter
			serverReceivePointsVec.WithLabelValues(pt).Inc()

			if pt == util.ProbeHTTPType {
				containers = httpAggMap
				phase = pr.HTTPFields
				key = util.JoinKey(
					pr.SourceRegion,
					pr.TargetRegion,
					pr.ProberTarget,
				)
			} else {
				containers = icmpAggMap
				phase = pr.ICMPFields
				key = util.JoinKey(
					pr.SourceRegion,
					pr.TargetRegion,
				)
			}

			if _, ok := containers[key]; !ok {
				containers[key] = &aggProberResult{
					sourceRegion: pr.SourceRegion,
					targetRegion: pr.TargetRegion,
					targetAddr:   pr.ProberTarget,
					phase:        make(map[string]float64),
					failedReason: httpDefaultFailedReason,
				}
			}

			// 只有 ProberSuccess 成功的任务才 batchCnt++
			// 保证算agg时，分母一定为成功的job数
			// 防止: 成功4台,失败1台；算agg: 理想 total/4, 结果 total/5, 反而会拉低实际值
			container := containers[key]
			if pr.ProberSuccess {
				container.batchCnt++

				// 仅累加成功任务的phase
				for stage, val := range phase {
					container.phase[stage] += val
				}

				// 处理tls
				if len(pr.TLSFields) > 0 && container.tlsExpiry == 0 && len(container.tlsVersion) == 0 {
					container.tlsVersion = pr.TLSFields["version"]
					expiry, _ := strconv.ParseInt(pr.TLSFields["expiry"], 10, 64)
					container.tlsExpiry = expiry
				}
				continue
			}
			// 走到下面逻辑，说明当前探测失败

			// 为http设定failed
			if pt == util.ProbeHTTPType && len(pr.ProberFailedReason) > 0 && container.failedReason == httpDefaultFailedReason {
				// 如果探测类型是 http ，并且当前存在失败信息，并且 failedReason还未初始化信息，这种情况下才去赋值
				// 也就是说 这里只会获取第一次获取到的拨测失败信息
				container.failedReason = pr.ProberFailedReason
			}

			// 失败任务 failedCnt自增
			container.failedCnt++
		}
	}

	a.dotHTTP(httpAggMap)
	a.dotICMP(icmpAggMap)
}

func (a *Aggregator) dotHTTP(http map[string]*aggProberResult) {
	for k := range http {
		agg := http[k]

		if len(agg.tlsVersion) > 0 && agg.tlsExpiry != 0 {
			ks := []string{
				agg.sourceRegion,
				agg.targetRegion,
				agg.targetAddr,
				agg.tlsVersion,
			}
			httpSSLEarliestCertExpiryGaugeVec.WithLabelValues(ks...).Set(float64(agg.tlsExpiry))
			a.setCache(a.httpMetricsHold, ks...)
		}

		ks := []string{
			agg.sourceRegion,
			agg.targetRegion,
			agg.targetAddr,
			agg.failedReason,
		}
		httpProberFailedGaugeVec.WithLabelValues(ks...).Set(float64(agg.failedCnt))

		// reset http httpProberFailedGaugeVec指标的缓存
		// 为什么要使用cache缓存，因为reason指标有状态，当reason过期是，需要删除old series；否则当前key的记录会一直被暴露
		a.setCache(a.httpMetricsHold, ks...)

		for stage, total := range agg.phase {
			ks := []string{
				stage,
				agg.sourceRegion,
				agg.targetRegion,
				agg.targetAddr,
			}

			// 每个 sR->tR 的每个stage的平均
			httpProberDurationGaugeVec.WithLabelValues(ks...).Set(total / float64(agg.batchCnt))
			// key不同(stage),需要另存一个key
			a.setCache(a.httpMetricsHold, ks...)
		}
	}
}

func (a *Aggregator) dotICMP(icmp map[string]*aggProberResult) {
	for k := range icmp {
		agg := icmp[k]
		ks := []string{
			agg.sourceRegion,
			agg.targetRegion,
		}

		// 当前 r to r 存在探测失败任务,记录失败次数；没有失败的，打点0
		icmpProberFailedGaugeVec.WithLabelValues(ks...).Set(float64(agg.failedCnt))

		// cache icmp的key
		a.setCache(a.icmpMetricsHold, ks...)

		var icmpDurationsTotal float64
		for stage, total := range agg.phase {
			stageAgg := total / float64(agg.batchCnt)

			switch stage {
			case "loss":
				// 单独打点丢包率指标
				icmpProberPacketLossRateGaugeVec.WithLabelValues(ks...).Set(stageAgg)
			case "stddev":
				// 单独打点 stddev 抖动指标
				icmpProberJitterStdDevGaugeVec.WithLabelValues(ks...).Set(stageAgg)
			default:
				ks := []string{
					stage,
					agg.sourceRegion,
					agg.targetRegion,
				}
				// 每个 sR->tR 的每个stage的平均
				icmpProberDurationGaugeVec.WithLabelValues(ks...).Set(stageAgg)

				// 由于label不同(stage),所以要另存一个key
				a.setCache(a.icmpMetricsHold, ks...)

				icmpDurationsTotal += total
			}
		}

		// 为 r->r 打点histogram
		icmpProberDurationHistogramVec.WithLabelValues(ks...).Observe(icmpDurationsTotal)
	}
}

func (a *Aggregator) setCache(c *cache.Cache, ks ...string) {
	c.SetDefault(util.JoinKey(ks...), nil)
}

func (a *Aggregator) reset() {
	a.queue = make([][]*pb.PorberResultReq, 0)
}

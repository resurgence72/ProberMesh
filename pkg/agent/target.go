package agent

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"probermesh/pkg/config"
	"probermesh/pkg/pb"
	"probermesh/pkg/util"

	"github.com/sirupsen/logrus"
)

type targetManager struct {
	r               *rpcCli
	targets         map[string][]*config.ProberConfig
	refreshInterval time.Duration
	syncInterval    time.Duration
	ptsChan         chan *pb.ProberResultReq
	selfRegion      string
	ctx             context.Context

	ready, beforeReady chan struct{}

	targetsMu sync.RWMutex
}

var (
	tm            *targetManager
	proberTimeout time.Duration
)

func NewTargetManager(
	ctx context.Context,
	region string,
	pInterval,
	sInterval time.Duration,
	r *rpcCli,
	br chan struct{},
	ptsChan chan *pb.ProberResultReq,
) *targetManager {
	tm = &targetManager{
		targets:         make(map[string][]*config.ProberConfig),
		ptsChan:         ptsChan,
		refreshInterval: pInterval,
		syncInterval:    sInterval,
		r:               r,
		ctx:             ctx,
		ready:           make(chan struct{}),
		beforeReady:     br,
	}

	proberTimeout = time.Duration(math.Ceil(tm.refreshInterval.Seconds()*8/10)) * time.Second

	tm.selfRegion = getSelfRegion(region)
	return tm
}

func (t *targetManager) start() {
	<-t.beforeReady

	// 定时获取targets
	go util.Wait(t.ctx, t.syncInterval, func() {
		t.getTargets()

		if t.ready != nil {
			close(t.ready)
			t.ready = nil
		}
	})

	<-t.ready

	// 定时上报
	go t.batchSend()

	// 定时探测
	util.Wait(t.ctx, t.refreshInterval, t.prober)
}

func (t *targetManager) prober() {
	t.targetsMu.RLock()
	targets := t.targets
	t.targetsMu.RUnlock()

	for rgn, tg := range targets {
		for _, tt := range tg {
			pj := &proberJob{
				proberType:   tt.ProberType,
				targets:      tt.Targets,
				http:         tt.HttpProbe,
				icmp:         tt.ICMPProbe,
				sourceRegion: t.selfRegion,
				targetRegion: rgn,
				ch:           t.ptsChan,
				r:            t.r,
				ctx:          t.ctx,
			}
			go pj.run()
		}
	}
}

// batchSend 单goroutine消费探测结果channel并定时上报，避免共享slice的数据竞争；
// 上报失败时保留数据下轮重试；队列长度达到上限时丢弃新点并告警，防止内存无限增长
func (t *targetManager) batchSend() {
	ticker := time.NewTicker(t.refreshInterval)
	defer ticker.Stop()

	const maxPoints = 5000
	var pts []*pb.ProberResultReq

	for {
		select {
		case pt, ok := <-t.ptsChan:
			if !ok {
				return
			}
			if len(pts) >= maxPoints {
				logrus.Warnln("prober report queue is full, drop point")
				continue
			}
			pts = append(pts, pt)

		case <-ticker.C:
			if len(pts) == 0 {
				continue
			}

			if err := t.r.Call(
				"Server.ProberResultReport",
				pts,
				nil,
			); err != nil {
				logrus.Errorln("prober report failed ", err)
				// 上报失败，保留数据下轮重试
				continue
			}

			pts = pts[:0]

		case <-t.ctx.Done():
			return
		}
	}
}

func (t *targetManager) getTargets() {
	resp := new(pb.TargetPoolResp)
	err := t.r.Call(
		"Server.GetTargetPool",
		pb.TargetPoolReq{SourceRegion: t.selfRegion},
		resp,
	)
	if err != nil {
		logrus.Errorln("get targets failed ", err)
		return
	}

	var msg string
	for r, pcs := range resp.Targets {
		var batch int
		for i := range pcs {
			pc := pcs[i]
			batch += len(pc.Targets)

		}
		msg += fmt.Sprintf("[region == %s]|[targetLens == %d] ", r, batch)
	}
	logrus.Debugln("agent get current target list msg: ", msg)
	t.targetsMu.Lock()
	t.targets = resp.Targets
	t.targetsMu.Unlock()
}

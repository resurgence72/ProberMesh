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
	currents        map[string]struct{}
	ptsChan         chan *pb.PorberResultReq
	selfRegion      string
	ctx             context.Context

	ready, beforeReady chan struct{}

	m sync.Mutex
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
	ptsChan chan *pb.PorberResultReq,
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
	for region, tg := range t.targets {
		for _, tt := range tg {
			pj := &proberJob{
				proberType:   tt.ProberType,
				targets:      tt.Targets,
				http:         tt.HttpProbe,
				sourceRegion: t.selfRegion,
				targetRegion: region,
				ch:           t.ptsChan,
				r:            t.r,
			}
			go pj.run()
		}
	}
}

func (t *targetManager) batchSend() {
	var pts []*pb.PorberResultReq

	go func() {
		for pt := range t.ptsChan {
			pts = append(pts, pt)
		}
	}()

	util.Wait(t.ctx, t.refreshInterval, func() {
		if len(pts) == 0 {
			return
		}

		if err := t.r.Call(
			"Server.ProberResultReport",
			pts,
			nil,
		); err != nil {
			logrus.Errorln("prober report failed ", err)
		}

		pts = pts[:0]
	})
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

			compileProbeRegexp(&pc.HttpProbe)
		}
		msg += fmt.Sprintf("[region == %s]|[targetLens == %d] ", r, batch)
	}
	logrus.Debugln("agent get current target list msg: ", msg)
	t.targets = resp.Targets
}

func compileProbeRegexp(probe *config.HTTPProbe) {
	for i := range probe.FailIfBodyMatchesRegexp {
		regexp, _ := config.NewRegexp(probe.FailIfBodyMatchesRegexp[i].Original)
		probe.FailIfBodyMatchesRegexp[i] = regexp
	}

	for i := range probe.FailIfBodyNotMatchesRegexp {
		regexp, _ := config.NewRegexp(probe.FailIfBodyNotMatchesRegexp[i].Original)
		probe.FailIfBodyNotMatchesRegexp[i] = regexp
	}

	for i := range probe.FailIfHeaderMatchesRegexp {
		regexp, _ := config.NewRegexp(probe.FailIfHeaderMatchesRegexp[i].Regexp.Original)
		probe.FailIfHeaderMatchesRegexp[i].Regexp = regexp
	}

	for i := range probe.FailIfHeaderNotMatchesRegexp {
		regexp, _ := config.NewRegexp(probe.FailIfHeaderNotMatchesRegexp[i].Regexp.Original)
		probe.FailIfHeaderNotMatchesRegexp[i].Regexp = regexp
	}
}

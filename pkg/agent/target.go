package agent

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"math"
	"probermesh/config"
	"probermesh/pkg/pb"
	"probermesh/pkg/util"
	"time"
)

type targetManager struct {
	r               *rpcCli
	targets         map[string][]*config.ProberConfig
	refreshInterval time.Duration
	syncInterval    time.Duration
	currents        map[string]struct{}
	selfRegion      string

	ready, beforeReady chan struct{}
}

var (
	tm            *targetManager
	proberTimeout time.Duration
)

func NewTargetManager(
	region string,
	pInterval,
	sInterval time.Duration,
	r *rpcCli,
	br chan struct{},
) *targetManager {
	tm = &targetManager{
		targets:         make(map[string][]*config.ProberConfig),
		refreshInterval: pInterval,
		syncInterval:    sInterval,
		r:               r,
		ready:           make(chan struct{}),
		beforeReady:     br,
	}

	proberTimeout = time.Duration(math.Ceil(tm.refreshInterval.Seconds()*8/10)) * time.Second

	tm.selfRegion = getSelfRegion(region)
	return tm
}

func (t *targetManager) start(ctx context.Context) {
	<-t.beforeReady

	// 定时获取targets
	go util.Wait(ctx, t.syncInterval, func() {
		t.getTargets()

		if t.ready != nil {
			close(t.ready)
			t.ready = nil
		}
	})

	<-t.ready

	// 定时探测
	util.Wait(ctx, t.refreshInterval, t.prober)
}

func (t *targetManager) prober() {
	for region, tg := range t.targets {
		for _, tt := range tg {
			pj := &proberJob{
				proberType:   tt.ProberType,
				targets:      tt.Targets,
				sourceRegion: t.selfRegion,
				targetRegion: region,
				r:            t.r,
			}
			pj.run()
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
	for region, pcs := range resp.Targets {
		var batch int
		for _, pc := range pcs {
			batch += len(pc.Targets)
		}
		msg += fmt.Sprintf("[region == %s]|[targetLens == %d] ", region, batch)
	}
	logrus.Warnln("agent get current target list msg: ", msg)
	t.targets = resp.Targets
}

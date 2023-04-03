package agent

import (
	"context"
	"sync"
	"time"

	"probermesh/pkg/config"
	"probermesh/pkg/pb"
	"probermesh/pkg/util"

	"github.com/sirupsen/logrus"
)

type proberJob struct {
	proberType   string
	targets      []string
	sourceRegion string
	targetRegion string

	http config.HTTPProbe
	r    *rpcCli
	ch   chan *pb.PorberResultReq
}

func (p *proberJob) run() {
	ctx, _ := context.WithTimeout(context.TODO(), proberTimeout)
	pt := util.ProbeICMPType

	if p.proberType == util.ProbeHTTPType {
		pt = util.ProbeHTTPType
	}
	p.dispatch(ctx, pt)
}

func (p *proberJob) jobExist(
	proberType string,
	sourceRegion string,
	targetRegion string,
	proberTarget string,
) bool {
	key := util.JoinKey(
		proberType,
		sourceRegion,
		targetRegion,
		proberTarget,
	)

	tm.m.Lock()
	defer tm.m.Unlock()
	if tm.currents == nil {
		tm.currents = make(map[string]struct{})
	}

	if _, ok := tm.currents[key]; ok {
		return true
	}
	tm.currents[key] = struct{}{}
	return false
}

func (p *proberJob) dispatch(ctx context.Context, pType string) {
	var wg sync.WaitGroup

	for _, target := range p.targets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()

			// 防止重复地址探测
			if p.jobExist(
				pType,
				p.sourceRegion,
				p.targetRegion,
				target,
			) {
				logrus.Warnln("target deduplication ", pType, p.sourceRegion, p.targetRegion, target)
				return
			}

			<-time.After(time.Duration(util.SetJitter()) * time.Millisecond)
			if pType == util.ProbeICMPType {
				p.ch <- probeICMP(ctx, target, p.sourceRegion, p.targetRegion)
			} else {
				p.ch <- probeHTTP(ctx, target, p.http, p.sourceRegion, p.targetRegion)
			}
		}(target)
	}

	wg.Wait()
	tm.currents = nil
}

package agent

import (
	"context"
	"github.com/sirupsen/logrus"
	"probermesh/pkg/pb"
	"probermesh/pkg/util"
	"sync"
	"time"
)

type proberJob struct {
	proberType   string
	targets      []string
	sourceRegion string
	targetRegion string
	r            *rpcCli

	m sync.Mutex
}

func (p *proberJob) run() {
	ctx, _ := context.WithTimeout(context.TODO(), tm.refreshInterval)
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
	s := util.DefaultKeySeparator
	key := proberType + s + sourceRegion + s + targetRegion + s + proberTarget

	p.m.Lock()
	defer p.m.Unlock()
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
	var (
		ptsChan = make(chan *pb.PorberResultReq, len(p.targets))
		pts     = make([]*pb.PorberResultReq, 0, len(p.targets))
		wg      sync.WaitGroup
	)

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
				ptsChan <- probeICMP(ctx, target, p.sourceRegion, p.targetRegion)
			} else {
				ptsChan <- probeHTTP(ctx, target, p.sourceRegion, p.targetRegion)
			}
			time.Sleep(1 * time.Second)
		}(target)
	}

	go func() {
		wg.Wait()
		close(ptsChan)
		tm.currents = nil
	}()

	for pt := range ptsChan {
		pts = append(pts, pt)
	}

	// batch send
	go func(pts []*pb.PorberResultReq) {
		if err := p.r.Call(
			"Server.ProberResultReport",
			pts,
			nil,
		); err != nil {
			logrus.Errorln("prober report failed ", err)
		}
	}(pts)
}

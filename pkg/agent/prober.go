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

	http *config.HTTPProbe
	icmp *config.ICMPProbe

	r  *rpcCli
	ch chan *pb.ProberResultReq

	ctx context.Context
}

func (p *proberJob) run() {
	ctx, cancel := context.WithTimeout(p.ctx, proberTimeout)
	defer cancel()

	pt := util.ProbeICMPType

	if p.proberType == util.ProbeHTTPType {
		pt = util.ProbeHTTPType
	}
	p.dispatch(ctx, pt)
}

// send 安全发送探测结果：context取消或channel无法写入时立即返回，不阻塞
func (p *proberJob) send(ctx context.Context, res *pb.ProberResultReq) {
	select {
	case p.ch <- res:
	case <-ctx.Done():
	}
}

func (p *proberJob) dispatch(ctx context.Context, pType string) {
	// 防止重复地址探测（job内本地去重）
	seen := make(map[string]struct{})
	var targets []string
	for _, target := range p.targets {
		key := util.JoinKey(
			pType,
			p.sourceRegion,
			p.targetRegion,
			target,
		)
		if _, ok := seen[key]; ok {
			logrus.Warnln("target deduplication ", pType, p.sourceRegion, p.targetRegion, target)
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}

	var wg sync.WaitGroup

	for _, target := range targets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()

			<-time.After(time.Duration(util.SetJitter()) * time.Millisecond)
			if pType == util.ProbeICMPType {
				p.send(ctx, probeICMP(ctx, target, p.icmp, p.sourceRegion, p.targetRegion))
			} else {
				p.send(ctx, probeHTTP(ctx, target, p.http, p.sourceRegion, p.targetRegion))
			}
		}(target)
	}

	wg.Wait()
}

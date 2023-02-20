package server

import (
	"context"
	"github.com/patrickmn/go-cache"
	"probermesh/pkg/util"
	"sync"
	"time"
)

type healthDot struct {
	expires      time.Duration
	agentPool    *cache.Cache
	discoverPool map[string]map[string]struct{}

	cancel context.Context
	ready  chan struct{}
	m      sync.Mutex
}

var hd *healthDot

func newHealthDot(
	ctx context.Context,
	expires time.Duration,
	ratio int,
	ready chan struct{},
) *healthDot {
	cacheInterval := time.Duration(ratio) * expires

	hd = &healthDot{
		expires:      expires,
		agentPool:    cache.New(cacheInterval, cacheInterval),
		discoverPool: make(map[string]map[string]struct{}),
		cancel:       ctx,
		ready:        ready,
	}

	// 过期时，打点0
	hd.agentPool.OnEvicted(func(key string, i interface{}) {
		sks := util.SplitKey(key)
		agentHealthCheckGaugeVec.WithLabelValues(sks...).Set(0)

		hd.m.Lock()
		defer hd.m.Unlock()
		delete(hd.discoverPool[sks[0]], sks[1])
	})
	return hd
}

func (h *healthDot) report(region, ip, version string) {
	h.m.Lock()
	defer h.m.Unlock()

	key := util.JoinKey(region, ip, version)
	h.agentPool.SetDefault(key, nil)

	// 将上报的agent region和ip存入
	if ipm, ok := h.discoverPool[region]; ok {
		ipm[ip] = struct{}{}
		//ips = append(ips, ip)
	} else {
		h.discoverPool[region] = map[string]struct{}{ip: {}}
	}

	if h.ready != nil {
		close(h.ready)
		h.ready = nil
	}
}

func (h *healthDot) dot() {
	util.Wait(h.cancel, h.expires, func() {
		for key := range h.agentPool.Items() {
			agentHealthCheckGaugeVec.WithLabelValues(util.SplitKey(key)...).Set(1)
		}
	})
}

func getDiscoverPool() map[string]map[string]struct{} {
	if hd == nil {
		return map[string]map[string]struct{}{}
	}
	return hd.discoverPool
}

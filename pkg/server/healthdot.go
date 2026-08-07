package server

import (
	"context"
	"sync"
	"time"

	"probermesh/pkg/util"

	"github.com/patrickmn/go-cache"
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
		version, _ := i.(string)
		agentHealthCheckGaugeVec.WithLabelValues(sks[0], sks[1], version).Set(0)

		hd.m.Lock()
		defer hd.m.Unlock()
		delete(hd.discoverPool[sks[0]], sks[1])
	})
	return hd
}

func (h *healthDot) report(region, ip, version string) {
	h.m.Lock()
	defer h.m.Unlock()

	// key不包含version，agent升级时同key覆盖刷新缓存，避免过期事件误删在线agent
	key := util.JoinKey(region, ip)
	h.agentPool.SetDefault(key, version)

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
		h.m.Lock()
		defer h.m.Unlock()

		for key, item := range h.agentPool.Items() {
			sks := util.SplitKey(key)
			version, _ := item.Object.(string)
			agentHealthCheckGaugeVec.WithLabelValues(sks[0], sks[1], version).Set(1)
		}
	})
}

// getDiscoverPoolSnapshot 返回discoverPool的深拷贝快照，调用方安全遍历
func getDiscoverPoolSnapshot() map[string]map[string]struct{} {
	if hd == nil {
		return map[string]map[string]struct{}{}
	}

	hd.m.Lock()
	defer hd.m.Unlock()

	snapshot := make(map[string]map[string]struct{}, len(hd.discoverPool))
	for region, ipm := range hd.discoverPool {
		m := make(map[string]struct{}, len(ipm))
		for ip := range ipm {
			m[ip] = struct{}{}
		}
		snapshot[region] = m
	}
	return snapshot
}

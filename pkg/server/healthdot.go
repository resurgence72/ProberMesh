package server

import (
	"context"
	"probermesh/pkg/util"
	"strings"
	"sync"
	"time"
)

type healthDot struct {
	expires      time.Duration
	agentPool    map[string]time.Time
	discoverPool map[string]map[string]struct{}

	cancel context.Context
	ready  chan struct{}
	m      sync.Mutex
}

var hd *healthDot

func newHealthDot(ctx context.Context, expires time.Duration, ready chan struct{}) *healthDot {
	hd = &healthDot{
		expires:      expires,
		agentPool:    make(map[string]time.Time),
		discoverPool: make(map[string]map[string]struct{}),
		cancel:       ctx,
		ready:        ready,
	}
	return hd
}

func (h *healthDot) report(region, ip, version string) {
	h.m.Lock()
	defer h.m.Unlock()
	h.agentPool[region+defaultKeySeparator+ip+defaultKeySeparator+version] = time.Now()

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
		now := time.Now()
		for agent, tm := range h.agentPool {
			meta := strings.Split(agent, defaultKeySeparator)
			region, ip, version := meta[0], meta[1], meta[2]
			if now.Sub(tm) > h.expires {
				agentHealthCheckGaugeVec.WithLabelValues(region, ip, version).Set(0)

				h.m.Lock()
				delete(h.agentPool, agent)
				delete(h.discoverPool[region], ip)
				h.m.Unlock()
			} else {
				agentHealthCheckGaugeVec.WithLabelValues(region, ip, version).Set(1)
			}
		}
	})
}

func getDiscoverPool() map[string]map[string]struct{} {
	if hd == nil {
		return map[string]map[string]struct{}{}
	}
	return hd.discoverPool
}

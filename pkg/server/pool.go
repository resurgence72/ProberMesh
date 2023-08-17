package server

import (
	"context"
	"fmt"
	"time"

	"probermesh/pkg/config"
	"probermesh/pkg/util"

	"github.com/sirupsen/logrus"
)

type targetsPool struct {
	pool             map[string]map[string]*config.ProberConfig
	cfgFn            func() *config.ProberMeshConfig
	reloadCh         chan chan error
	discovery        discoveryType
	probeSelfEnabled bool

	done  context.Context
	ready chan struct{}
}

type discoveryType string

const (
	StaticDiscovery  discoveryType = "static"
	DynamicDiscovery discoveryType = "dynamic"
)

var tp *targetsPool

func newTargetsPool(
	ctx context.Context,
	cfgFn func() *config.ProberMeshConfig,
	reloadCh chan chan error,
	ready chan struct{},
	dt string,
	probeSelfEnabled bool,
) *targetsPool {
	discovery := discoveryType(dt)

	tp = &targetsPool{
		pool:             make(map[string]map[string]*config.ProberConfig),
		cfgFn:            cfgFn,
		reloadCh:         reloadCh,
		probeSelfEnabled: probeSelfEnabled,
		done:             ctx,
		ready:            ready,
		discovery:        discovery,
	}

	switch discovery {
	case DynamicDiscovery:
		logrus.Warnln("server use dynamic discovery type to find icmp targets by agent report")
		go tp.updatePool()
	case StaticDiscovery:
		logrus.Warnln("server use static discovery type to find icmp targets by config")
	}
	return tp
}

func (t *targetsPool) start() {
	t.loadPool()
	<-t.done.Done()
	return
}

func (t *targetsPool) reloadPool() error {
	t.loadPool()
	return nil
}

func (t *targetsPool) loadPool() {
	fmt.Println("loadPool ", t.cfgFn().ProberConfigs[0].Region)
	for _, pc := range t.cfgFn().ProberConfigs {
		/*
			{
				cn-shanghai: {
					icmp: icmpConfig,
					http: httpConfig,
				}
			}
		*/
		// dynamic情况下忽略配置文件icmp target
		// if t.discovery == DynamicDiscovery && pc.ProberType == util.ProbeICMPType {
		//	continue
		// }

		if pcm, ok := t.pool[pc.Region]; ok {
			pcm[pc.ProberType] = pc
		} else {
			t.pool[pc.Region] = map[string]*config.ProberConfig{pc.ProberType: pc}
		}
	}
}

func (t *targetsPool) updatePool() {
	var first = true

	// 每分钟更新次pool值(根据agent上报)
	util.Wait(t.done, time.Duration(1)*time.Minute, func() {
		var updateKey = util.ProbeICMPType
		for region, ipm := range getDiscoverPool() {
			if len(ipm) == 0 {
				// 当前region agent下线,则删除当前region下的icmp，防止后续继续同步
				if pm, ok := t.pool[region]; ok {
					if len(pm) > 0 {
						delete(t.pool[region], updateKey)
					} else {
						// 如果当前region下agent全部下线，则在pool中删除掉当前region信息
						delete(t.pool, region)
					}
				}
				continue
			}

			var ips []string
			for k := range ipm {
				ips = append(ips, k)
			}

			updatePC := &config.ProberConfig{
				ProberType: updateKey,
				Region:     region,
				Targets:    ips,
			}

			// 如果使用自动发现方式，则会覆盖掉配置中指定的同region下的icmp targets节点
			// 全部使用agent上报的ip进行探测
			if pm, ok := t.pool[region]; ok {
				pm[updateKey] = updatePC
			} else {
				t.pool[region] = map[string]*config.ProberConfig{updateKey: updatePC}
			}
		}

		if first {
			<-t.ready
			first = false
		}
	})
}

func (t *targetsPool) getPool(sourceRegion string) map[string][]*config.ProberConfig {
	pcs := make(map[string][]*config.ProberConfig)
	for region, pcm := range t.pool {
		// 不允许拨测自身 且 目的region和自身region相同时，跳过
		if !t.probeSelfEnabled && region == sourceRegion {
			continue
		}
		var ps []*config.ProberConfig
		for _, pc := range pcm {
			ps = append(ps, pc)
		}
		pcs[region] = ps
	}
	return pcs
}

func (t *targetsPool) getTargets() interface{} {
	type targetGroup struct {
		ProberType string   `json:"prober_type"`
		Targets    []string `json:"targets"`
	}

	pcs := make(map[string][]targetGroup)
	for region, pcm := range t.pool {
		var ps []targetGroup
		for _, pc := range pcm {
			ps = append(ps, targetGroup{
				ProberType: pc.ProberType,
				Targets:    pc.Targets,
			})
		}
		pcs[region] = ps
	}
	return pcs
}

package agent

import (
	"context"
	"github.com/oklog/run"
	"github.com/sirupsen/logrus"
	"log"
	"probermesh/pkg/util"
	"time"
)

type ProberMeshAgentOption struct {
	Addr,
	PInterval,
	SInterval,
	UInterval,
	NetworkType,
	Region string
	Upgrade bool
}

func BuildAgentMode(ao *ProberMeshAgentOption) {
	if len(ao.Addr) == 0 {
		log.Fatal("server addr must be set")
	}

	switch ao.NetworkType {
	case intranetNetType, publicNetType:
	default:
		log.Fatal("agent network type must one of intranet or public")
	}
	initAgentLocalIP(ao.NetworkType)

	pDuration, err := util.ParseDuration(ao.PInterval)
	if err != nil {
		logrus.Errorln("parse prober duration flag failed ", err)
		return
	}

	sDuration, err := util.ParseDuration(ao.SInterval)
	if err != nil {
		logrus.Errorln("parse sync duration flag failed ", err)
		return
	}

	UInterval, err := util.ParseDuration(ao.UInterval)
	if err != nil {
		logrus.Errorln("parse sync duration flag failed ", err)
		return
	}

	ctxAll, cancelAll := context.WithCancel(context.Background())

	cli := initRpcCli(ctxAll, ao.Addr)
	var g run.Group

	// 上报后再manager.start()
	beforeReady := make(chan struct{})
	{
		// 定时拉取mesh poll
		manager := NewTargetManager(
			ao.Region,
			pDuration,
			sDuration,
			cli,
			beforeReady,
		)
		g.Add(func() error {
			manager.start(ctxAll)
			return nil
		}, func(err error) {
			cancelAll()
		})
	}

	{
		// healthCheck
		g.Add(func() error {
			newHealthCheck(ctxAll, cli, beforeReady).report()
			return nil
		}, func(e error) {
			cancelAll()
		})
	}

	{
		if ao.Upgrade {
			logrus.Warnln("open agent self upgrade functions")
			// upgrade
			g.Add(func() error {
				util.Wait(
					ctxAll,
					UInterval,
					newUpgradeChecker(cli).startUpgradeCheck,
				)
				return nil
			}, func(e error) {
				cancelAll()
			})
		}
	}

	g.Run()
}

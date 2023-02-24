package agent

import (
	"context"
	"time"

	"probermesh/pkg/pb"
	"probermesh/pkg/util"

	"github.com/sirupsen/logrus"
)

const (
	healthCheckInterval = time.Duration(10) * time.Second
)

type healthCheck struct {
	r          *rpcCli
	selfRegion string
	selfAddr   string
	ready      chan struct{}

	cancel context.Context
}

func newHealthCheck(ctx context.Context, r *rpcCli, ready chan struct{}) *healthCheck {
	return &healthCheck{
		r:          r,
		selfRegion: tm.selfRegion,
		selfAddr:   agentIP,
		cancel:     ctx,
		ready:      ready,
	}
}

func (h *healthCheck) report() {
	util.Wait(h.cancel, healthCheckInterval, func() {
		var msg string
		err := h.r.Call(
			"Server.Report",
			pb.ReportReq{
				IP:      h.selfAddr,
				Region:  h.selfRegion,
				Version: util.GetVersion(),
			},
			&msg,
		)
		if err != nil {
			logrus.Errorln("rpc report failed ", err)
		}

		if h.ready != nil {
			close(h.ready)
			h.ready = nil
		}
	})
}

package agent

import (
	"github.com/sirupsen/logrus"
	"probermesh/pkg/pb"
	"probermesh/pkg/upgrade"
	"probermesh/pkg/util"
)

type upgradeChecker struct {
	r       *rpcCli
	version string
}

func newUpgradeChecker(r *rpcCli) *upgradeChecker {
	return &upgradeChecker{r: r, version: util.GetVersion()}
}

func (u *upgradeChecker) startUpgradeCheck() {
	resp := new(pb.UpgradeResp)
	req := pb.UpgradeCheckReq{
		Version: util.GetVersion(),
		Ident:   util.JoinKey(region, agentIP),
	}

	if err := u.r.Call(
		"Server.GetSelfUpgrade",
		req,
		resp,
	); err != nil {
		logrus.Errorln("Server.GetSelfUpgrade rpc call failed ", err)
		return
	}

	if resp.Upgraded {
		logrus.Warnln("agent begin start and check for upgraded")
		if err := upgrade.Upgrade(
			resp.DownloadURL,
			resp.Md5Check,
			func() error {
				// rpc上报更新成功的函数
				return u.r.Call(
					"Server.SelfUpgradeSuccess",
					req,
					nil,
				)
			}); err != nil {
			logrus.Errorln("agent upgrade failed, retrying...", err)
			return
		}
	}
}

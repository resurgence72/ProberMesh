package server

import (
	"bufio"
	"io"
	"net"
	"net/rpc"
	"reflect"
	"time"

	"probermesh/pkg/pb"
	"probermesh/pkg/upgrade"

	"github.com/sirupsen/logrus"
	"github.com/ugorji/go/codec"
)

type Server struct{}

// agent 健康检查上报
func (s *Server) Report(req pb.ReportReq, resp *string) error {
	hd.report(req.Region, req.IP, req.Version)
	return nil
}

// agent 获取target pool
func (s *Server) GetTargetPool(req pb.TargetPoolReq, resp *pb.TargetPoolResp) error {
	tcs := tp.getPool(req.SourceRegion)
	resp.Targets = tcs
	return nil
}

// agent 上报探测结果
func (s *Server) ProberResultReport(reqs []*pb.ProberResultReq, resp *string) error {
	aggregator.Enqueue(reqs)
	return nil
}

// agent 获取更新接口
func (s *Server) GetSelfUpgrade(req pb.UpgradeCheckReq, resp *pb.UpgradeResp) error {
	u := upgrade.GetSelfUpgrade()
	if !u.Force {
		// 非强制更新，需检查version
		if !u.CheckVersion(u.Version, req.Version) {
			// 新版本<=当前版本
			return nil
		}
	} else {
		// 强制更新流程
		// agent升级完成，直接返回，防止重复升级
		if u.AgentIsUpgraded(req.Ident) {
			return nil
		}
	}

	resp.Upgraded = true
	resp.Md5Check = u.Md5Check
	resp.DownloadURL = u.DownloadURL
	return nil
}

// agent 更新成功上报接口
func (s *Server) SelfUpgradeSuccess(req pb.UpgradeCheckReq, resp *string) error {
	// 升级成功后，存入map
	u := upgrade.GetSelfUpgrade()
	u.AgentSetUpgraded(req.Ident)
	return nil
}

// agent 定时获取任务接口
func (s *Server) GetSelfTask(req string, resp *string) error {
	*resp = GetTaskGroup().getTask(req)
	return nil
}

// agent 上报任务结果
func (s *Server) ReportTaskResult(req pb.ReportTaskResultReq, resp *string) error {
	logrus.Warnln("server get agent task report:", req.Ident, req.Cmd)
	go GetTaskGroup().flushDisk(req)
	return nil
}

func startRpcServer(addr string) error {
	server := rpc.NewServer()
	err := server.Register(new(Server))
	if err != nil {
		return err
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	go func() {
		for {
			// 从accept中拿到一个客户端的连接
			conn, err := l.Accept()
			if err != nil {
				logrus.Errorln("listen accept failed ", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// 用bufferio做io解析提速
			var bufConn = struct {
				io.Closer
				*bufio.Reader
				*bufio.Writer
			}{
				conn,
				bufio.NewReader(conn),
				bufio.NewWriter(conn),
			}
			go server.ServeCodec(codec.MsgpackSpecRpc.ServerCodec(bufConn, &mh))
		}
	}()
	return nil
}

package server

import (
	"bufio"
	"github.com/sirupsen/logrus"
	"github.com/ugorji/go/codec"
	"io"
	"net"
	"net/rpc"
	"probermesh/pkg/pb"
	"probermesh/pkg/upgrade"
	"reflect"
	"time"
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
func (s *Server) ProberResultReport(reqs []*pb.PorberResultReq, resp *string) error {
	aggregator.Enqueue(reqs)
	return nil
}

// agent 获取更新接口
func (s *Server) GetSelfUpgrade(version pb.UpgradeCheckReq, resp *pb.UpgradeResp) error {
	u := upgrade.GetSelfUpgrade()

	if !u.Force {
		// 如果强制更新，跳过version的校验 回退
		if !u.CheckVersion(u.Version, version.String()) {
			// 新版本<=当前版本
			return nil
		}
	}

	resp = &pb.UpgradeResp{
		Upgraded:    true,
		Md5Check:    u.Md5Check,
		DownloadURL: u.DownloadURL,
	}
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

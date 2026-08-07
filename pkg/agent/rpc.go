package agent

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/rpc"
	"reflect"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/toolkits/pkg/net/gobrpc"
	"github.com/ugorji/go/codec"
)

type rpcCli struct {
	cli        *gobrpc.RPCClient
	serverAddr string
	timeout    time.Duration
	retry      int

	cancel context.Context

	mu sync.Mutex
}

func initRpcCli(ctx context.Context, addr string) *rpcCli {
	rc := &rpcCli{
		serverAddr: addr,
		cancel:     ctx,
		timeout:    time.Duration(3) * time.Second,
		retry:      3,
	}
	return rc
}

func (r *rpcCli) Call(sn string, req, resp interface{}) error {
	var err error

	cli := r.get()
	if cli == nil {
		err = errors.New("rpc client connect failed")
		logrus.Errorln("rpc call failed ", sn, err)
		return err
	}

	// retry
	for i := 1; i < r.retry+1; i++ {
		err = cli.Call(
			sn,
			req,
			resp,
		)
		if err == nil {
			return nil
		}
		time.Sleep(time.Duration(i) * 300 * time.Millisecond)
	}

	logrus.Errorln("rpc call failed ", sn, err)
	r.closeCli()
	return err
}

func (r *rpcCli) get() *gobrpc.RPCClient {
	cli, err := r.getCli()
	if err == nil && cli != nil {
		return cli
	}
	return nil
}

func (r *rpcCli) getCli() (*gobrpc.RPCClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cli != nil {
		return r.cli, nil
	}

	conn, err := net.DialTimeout("tcp", r.serverAddr, r.timeout)
	if err != nil {
		logrus.Errorln("get cli failed ", err)
		return nil, err
	}

	var bufConn = struct {
		io.Closer
		*bufio.Reader
		*bufio.Writer
	}{
		conn,
		bufio.NewReader(conn),
		bufio.NewWriter(conn),
	}

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	rpcCodec := codec.MsgpackSpecRpc.ClientCodec(bufConn, &mh)
	r.cli = gobrpc.NewRPCClient(
		r.serverAddr,
		rpc.NewClientWithCodec(rpcCodec),
		r.timeout,
	)
	return r.cli, nil
}

func (r *rpcCli) closeCli() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cli != nil {
		r.cli.Close()
		r.cli = nil
	}
}

package agent

import (
	"bufio"
	"context"
	"github.com/sirupsen/logrus"
	"io"
	"os/exec"
	"probermesh/pkg/pb"
	"time"
)

func taskRun(cmd string, ident string, cli *rpcCli) {
	ctx, _ := context.WithTimeout(context.TODO(), 30*time.Second)
	c := exec.CommandContext(ctx, "/bin/bash", "-c", cmd, " 2>&1")

	var (
		output string
	)
	defer func() {
		var s string
		_ = cli.Call(
			"Server.ReportTaskResult",
			pb.ReportTaskResultReq{
				Ident:  ident,
				Cmd:    cmd,
				Result: output,
			},
			&s,
		)
	}()

	logrus.Warnln("start run cmd: ", c.String())
	outReader, err := c.StdoutPipe()
	if err != nil {
		output = err.Error()
		return
	}

	c.Start()
	reader := bufio.NewReader(outReader)
	for {
		readString, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			output = err.Error()
			return
		}
		output += readString
	}
	c.Wait()
}

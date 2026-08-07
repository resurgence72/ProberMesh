package agent

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"time"

	"probermesh/pkg/pb"

	"github.com/sirupsen/logrus"
)

const maxTaskOutputSize = 64 * 1024

func taskRun(cmd string, ident string, cli *rpcCli) {
	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

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

	if err := c.Start(); err != nil {
		output = err.Error()
		return
	}

	reader := bufio.NewReader(outReader)
	for {
		readString, err := reader.ReadString('\n')
		if len(output) < maxTaskOutputSize {
			output += readString
			if len(output) > maxTaskOutputSize {
				output = output[:maxTaskOutputSize] + "\n...[output truncated]"
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			output = err.Error()
			break
		}
	}
	// 回收子进程，防止僵尸进程
	_ = c.Wait()
}

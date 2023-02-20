package server

import (
	"encoding/json"
	"fmt"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"probermesh/pkg/pb"
	"regexp"
	"strings"
	"time"
)

type expr string

const (
	Match         expr = "="
	NotMatch      expr = "!="
	RegexMatch    expr = "=~"
	RegexNotMatch expr = "!~"

	EmptyCmd string = ""
)

var taskgroup *TaskGroup

type TaskGroup struct {
	tasks *cache.Cache
	cmd   *exec.Cmd

	meteDir string
	enabled bool
}

func GetTaskGroup() *TaskGroup {
	return taskgroup
}

func newTaskGroup(enabled bool, dir string) *TaskGroup {
	cacheInterval := time.Duration(10) * time.Second
	hmh := cache.New(cacheInterval, cacheInterval)
	taskgroup = &TaskGroup{tasks: hmh, meteDir: dir, enabled: enabled}
	return taskgroup
}

func (tg *TaskGroup) task(r *http.Request) []byte {
	jm := func(v interface{}) []byte {
		bs, _ := json.Marshal(v)
		return bs
	}

	defaultResp := resp{
		Code: defaultCode,
		Msg:  "success",
		Data: "success",
	}

	t := new(pb.TaskReq)
	if err := json.NewDecoder(r.Body).Decode(t); err != nil {
		logrus.Errorln("task req json decode failed", err)
		return jm(resp{
			Code: defaultCode,
			Msg:  "task req json decode failed",
			Data: err.Error(),
		})
	}

	compile, err := regexp.Compile(t.Region)
	if err != nil {
		logrus.Errorln("regexp.Compile failed", err)
		return jm(resp{
			Code: defaultCode,
			Msg:  "regexp.Compile failed",
			Data: err.Error(),
		})
	}

	t.RegionReg = compile

	tg.tasks.SetDefault(t.Region, t)
	return jm(defaultResp)
}

func (tg *TaskGroup) getTask(ident string) string {
	if !tg.enabled {
		return EmptyCmd
	}

	for k := range tg.tasks.Items() {
		v, _ := tg.tasks.Get(k)
		tr, _ := v.(*pb.TaskReq)

		if tg.isMatch(ident, tr) {
			return tr.Cmd
		}
	}
	return EmptyCmd
}

func (tg *TaskGroup) isMatch(ident string, tr *pb.TaskReq) bool {
	switch expr(tr.Expr) {
	case Match:
		if strings.EqualFold(ident, tr.Expr) {
			return true
		}
	case NotMatch:
		if !strings.EqualFold(ident, tr.Expr) {
			return true
		}
	case RegexMatch:
		if tr.RegionReg.MatchString(ident) {
			return true
		}
	case RegexNotMatch:
		if !tr.RegionReg.MatchString(ident) {
			return true
		}
	default:
	}
	return false
}

func (tg *TaskGroup) flushDisk(r pb.ReportTaskResultReq) {
	_, err := os.Stat(tg.meteDir)
	if err != nil {
		if !os.IsExist(err) {
			_ = os.Mkdir(tg.meteDir, 0644)
		}
		logrus.Errorln("dir check failed:", err)
		return
	}

	content := fmt.Sprintf("commond:\n%s \n\noutput:\n%s", r.Cmd, r.Result)
	fn := tg.meteDir + r.Ident + "_" + time.Now().Format("2006-01-02_15-04-05")

	if err := ioutil.WriteFile(
		fn,
		[]byte(content),
		0644,
	);
		err != nil {
		logrus.Errorln("flush file failed:", err)
	}
}

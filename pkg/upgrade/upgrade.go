package upgrade

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"

	"probermesh/pkg/util"

	"github.com/sirupsen/logrus"
)

const (
	binaryPath = "./"
)

type SelfUpgrade struct {
	Md5Check    string `json:"md5Check"`
	Version     string `json:"version"`
	DownloadURL string `json:"downloadURL"`

	Force bool `json:"force"`

	m sync.RWMutex
}

type hook func() error

var (
	su         = newSelfUpgrade()
	UpgradeMap map[string]struct{}
)

func GetSelfUpgrade() *SelfUpgrade {
	su.m.Lock()
	defer su.m.Unlock()

	// lazy load
	if UpgradeMap == nil {
		UpgradeMap = make(map[string]struct{})
	}
	return su
}

func newSelfUpgrade() *SelfUpgrade {
	return &SelfUpgrade{Version: util.GetVersion()}
}

func (s *SelfUpgrade) CheckVersion(new, old string) bool {
	if strings.Compare(new, old) < 1 {
		return false
	}
	return true
}

func (s *SelfUpgrade) Bind(r *http.Request) error {
	s.m.Lock()
	defer s.m.Unlock()

	// 每次升级请求默认都需要重新下发，清空hold点的map
	for k := range UpgradeMap {
		delete(UpgradeMap, k)
	}
	return json.NewDecoder(r.Body).Decode(s)
}

func (s *SelfUpgrade) AgentIsUpgraded(ident string) bool {
	s.m.RLock()
	defer s.m.RUnlock()
	_, ok := UpgradeMap[ident]
	return ok
}

func (s *SelfUpgrade) AgentSetUpgraded(ident string) {
	s.m.Lock()
	defer s.m.Unlock()
	UpgradeMap[ident] = struct{}{}
}

func Upgrade(u, m string, hooks ...hook) error {
	defer func() {
		for i := range hooks {
			if err := hooks[i](); err != nil {
				logrus.WithFields(logrus.Fields{
					"idx":   i,
					"error": err,
				}).Errorln("upgrade pre kill hook func run failed")
			}
		}

		// kill
		logrus.Warnln("start kill old process")
		pro, _ := os.FindProcess(os.Getpid())
		pro.Signal(syscall.SIGTERM)
	}()

	// 下载二进制
	logrus.Warnln("start download new version binary file")
	resp, err := http.Get(u)
	if err != nil {
		return errors.New("upgrade http get failed")
	}
	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.New("upgrade ioutil ReadAll failed")
	}

	// 校验二进制的md5
	logrus.Warnln("start check md5sum for binary file")
	if strings.Compare(util.GetMd5(string(bs)), m) != 0 {
		io.Copy(ioutil.Discard, resp.Body)
		return errors.New("upgrade md5Check failed")
	}

	// 校验成功，删除原本二进制
	p := binaryPath + util.ProjectName
	os.Remove(p)

	// 新二进制写入本地
	logrus.Warnln("start replace local binary file")
	err = ioutil.WriteFile(p, bs, 0755)
	if err != nil {
		return errors.New("upgrade ioutil WriteFile failed")
	}

	return nil
}

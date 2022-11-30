package upgrade

import (
	"errors"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"probermesh/pkg/util"
	"strings"
	"sync"
	"syscall"
)

const (
	binaryPath = "./"
)

type SelfUpgrade struct {
	Md5Check    string `json:"md5Check"`
	Version     string `json:"version"`
	DownloadURL string `json:"downloadURL"`

	Force bool `json:"force"`

	m sync.Mutex
}

var su = newSelfUpgrade()

func GetSelfUpgrade() *SelfUpgrade {
	su.m.Lock()
	defer su.m.Unlock()
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

func Upgrade(u, m string) error {
	// 下载二进制
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
	if strings.Compare(util.GetMd5(string(bs)), m) != 0 {
		io.Copy(ioutil.Discard, resp.Body)
		return errors.New("upgrade md5Check failed")
	}

	// 校验成功，删除原本二进制
	p := binaryPath + util.ProjectName
	os.Remove(p)

	// 新二进制写入本地
	err = ioutil.WriteFile(p, bs, 0755)
	if err != nil {
		return errors.New("upgrade ioutil WriteFile failed")
	}
	logrus.Warnln("upgrade check success, upgrading")

	// kill
	defer func() {
		pro, _ := os.FindProcess(os.Getpid())
		pro.Signal(syscall.SIGTERM)
	}()
	return nil
}

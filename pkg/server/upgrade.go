package server

import (
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"probermesh/pkg/util"
	"strings"
	"syscall"
)

const (
	binaryPath = "./"
	binaryName = "proberMesh-test"
)

func upgrade() func(http.ResponseWriter, *http.Request) {
	u := &struct {
		Md5Check    string `json:"md5Check"`
		Version     string `json:"version"`
		DownloadURL string `json:"downloadURL"`
	}{}

	return func(w http.ResponseWriter, r *http.Request) {
		//w.Header().Set("Content-Type", "application-json")
		err := json.NewDecoder(r.Body).Decode(u)
		if err != nil {
			logrus.Errorln("upgrade json decode failed", err)
			w.Write([]byte(err.Error()))
			return
		}

		// 校验version是否需要升级
		if !checkVersion(u.Version, util.GetVersion()) {
			// 新版本<=当前版本
			logrus.Warnln("new version lte old version, pass")
			w.Write([]byte("new version lte old version, pass"))
			return
		}

		// 校验http是否正确
		uri, err := url.ParseRequestURI(u.DownloadURL)
		if err != nil {
			logrus.Errorln("upgrade ParseRequestURI failed", err)
			w.Write([]byte(fmt.Sprintf("[%s] [%s]", err.Error(), u.DownloadURL)))
			return
		}

		// 下载二进制
		resp, err := http.Get(uri.String())
		if err != nil {
			logrus.Errorln("upgrade http get failed", err)
			w.Write([]byte(err.Error()))
			return
		}
		defer resp.Body.Close()

		bs, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logrus.Errorln("upgrade ioutil ReadAll failed", err)
			w.Write([]byte(err.Error()))
			return
		}

		// 校验二进制的md5
		m := util.GetMd5(string(bs))
		if strings.Compare(m, u.Md5Check) != 0 {
			logrus.Errorln("upgrade md5Check failed")
			w.Write([]byte("upgrade md5Check failed"))
			return
		}

		// 校验成功，删除原本二进制
		p := binaryPath + binaryName
		os.Remove(p)

		// 新二进制写入本地
		err = ioutil.WriteFile(p, bs, 0644)
		if err != nil {
			logrus.Errorln("upgrade ioutil WriteFile failed", err)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write([]byte("upgrade check success, upgrading"))
		// kill
		killSelf()
	}
}

func checkVersion(new, old string) bool {
	if strings.Compare(new, old) < 1 {
		return false
	}
	return true
}

func killSelf() {
	p, _ := os.FindProcess(os.Getpid())
	p.Signal(syscall.SIGTERM)
}

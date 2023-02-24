package server

import (
	"encoding/json"
	"net/http"
	"net/url"

	"probermesh/pkg/upgrade"

	"github.com/sirupsen/logrus"
)

type resp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
}

const defaultCode = 200

func update(r *http.Request) []byte {
	jm := func(v interface{}) []byte {
		bs, _ := json.Marshal(v)
		return bs
	}

	defaultResp := resp{
		Code: defaultCode,
		Msg:  "success",
		Data: "success",
	}

	u := upgrade.GetSelfUpgrade()
	if err := u.Bind(r); err != nil {
		logrus.Errorln("upgrade json decode failed", err)
		return jm(resp{
			Code: defaultCode,
			Msg:  "upgrade json decode failed",
			Data: err.Error(),
		})
	}

	// 校验http是否正确
	uri, err := url.ParseRequestURI(u.DownloadURL)
	if err != nil {
		logrus.Errorln("upgrade ParseRequestURI check failed", err)
		return jm(resp{
			Code: defaultCode,
			Msg:  "upgrade ParseRequestURI check failed",
			Data: err.Error(),
		})
	}
	logrus.Warnln("server check upgrade success, dispatch upgrade request...")

	u.DownloadURL = uri.String()
	return jm(defaultResp)
}

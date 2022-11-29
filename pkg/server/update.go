package server

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"probermesh/pkg/upgrade"
)

type resp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
}

const defaultCode = 200

func update() func(http.ResponseWriter, *http.Request) {
	jm := func(v interface{}) []byte {
		bs, _ := json.Marshal(v)
		return bs
	}

	defaultResp := resp{
		Code: defaultCode,
		Msg:  "success",
		Data: "success",
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		u := upgrade.GetSelfUpgrade()
		err := json.NewDecoder(r.Body).Decode(u)
		if err != nil {
			logrus.Errorln("upgrade json decode failed", err)
			w.Write(jm(resp{
				Code: defaultCode,
				Msg:  "upgrade json decode failed",
				Data: err.Error(),
			}))
			return
		}

		// 校验http是否正确
		uri, err := url.ParseRequestURI(u.DownloadURL)
		if err != nil {
			logrus.Errorln("upgrade ParseRequestURI check failed", err)
			w.Write(jm(resp{
				Code: defaultCode,
				Msg:  "upgrade ParseRequestURI check failed",
				Data: err.Error(),
			}))
			return
		}

		u.DownloadURL = uri.String()
		w.Write(jm(defaultResp))
	}
}

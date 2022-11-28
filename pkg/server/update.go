package server

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"probermesh/pkg/upgrade"
)

func update() func(http.ResponseWriter, *http.Request) {
	jm := func(v interface{}) []byte {
		bs, _ := json.Marshal(v)
		return bs
	}

	return func(w http.ResponseWriter, r *http.Request) {
		resp := &struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data string `json:"data"`
		}{
			Code: 200,
			Msg:  "success",
			Data: "success",
		}

		u := upgrade.GetSelfUpgrade()
		w.Header().Set("Content-Type", "application/json")
		err := json.NewDecoder(r.Body).Decode(u)
		if err != nil {
			logrus.Errorln("upgrade json decode failed", err)
			resp.Msg = "upgrade json decode failed"
			resp.Data = err.Error()
			w.Write(jm(resp))
			return
		}

		// 校验http是否正确
		uri, err := url.ParseRequestURI(u.DownloadURL)
		if err != nil {
			logrus.Errorln("upgrade ParseRequestURI check failed", err)
			resp.Msg = "upgrade ParseRequestURI check failed"
			resp.Data = err.Error()
			w.Write(jm(resp))
			return
		}

		u.DownloadURL = uri.String()
		w.Write(jm(resp))
	}
}

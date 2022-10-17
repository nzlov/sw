package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"go.uber.org/zap"
)

func adminresp(log *zap.SugaredLogger, w http.ResponseWriter, code, content string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"code":"` + code + `","data":"` + content + `"}`))
	log.Info("[ADMINRESP]", code, content)
}

func (n *Node) adminPush(w http.ResponseWriter, r *http.Request) {
	log := zap.S().With("method", "adminpush")
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		adminresp(log, w, C_FAIL, "读取数据错误")
		return
	}
	log.Info("[Admin]新的请求:", string(body))

	s := r.URL.Query().Get("sign")
	if s == "" {
		adminresp(log, w, C_FAIL, "sign")
		return
	}
	ts := r.URL.Query().Get("ts")
	if ts == "" {
		adminresp(log, w, C_FAIL, "ts")
		return
	}

	if !CheckSignMD5(DefConfig.AdminSecret, string(body), ts, s) {
		adminresp(log, w, C_FAIL, "sign")
		return
	}

	pm := AdminPushMessage{}
	if err := json.Unmarshal(body, &pm); err != nil {
		adminresp(log, w, C_FAIL, "data format")
		return
	}
	pm.MessageID = fmt.Sprint(time.Now().UnixNano())
	n.Publish(pm, false, 0)
	adminresp(log, w, C_OK, pm.MessageID)
}

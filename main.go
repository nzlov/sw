package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	_ "net/http/pprof"
)

func main() {
	log, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(log)
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath("./")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	err := viper.ReadInConfig()
	if err != nil {
		log.Sugar().Fatal("init config error:", err)
	}

	err = viper.Unmarshal(&DefConfig)
	if err != nil {
		log.Sugar().Fatal("init config unmarshal error:", err)
	}

	go func() {
		http.ListenAndServe(DefConfig.PprofHost, nil)
	}()

	node := newNode()
	defer node.Close()

	m := http.NewServeMux()
	m.HandleFunc("/", node.adminPush)
	m.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		node.serveWs(node, w, r)
	})
	log.Sugar().Info("Start:", DefConfig.Host)
	err = http.ListenAndServe(DefConfig.Host, m)
	if err != nil {
		log.Sugar().Fatal("ListenAndServe: ", err)
	}
	fmt.Println("close")
}

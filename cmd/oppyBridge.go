package main

import (
	"net/http"
	_ "net/http/pprof"

	golog "github.com/ipfs/go-log"
	"github.com/oppyfinance/tss/common"
	"github.com/rs/zerolog"
	"gitlab.com/oppy-finance/oppy-bridge/bridge"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

func main() {
	misc.SetupBech32Prefix()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	cfg := config.DefaultConfig()
	err := golog.SetLogLevel("tss-lib", "INFO")
	go func() {
		http.ListenAndServe("0.0.0.0:8888", nil)
	}()
	if err != nil {
		panic(err)
	}
	common.InitLog("info", true, "oppyBridge_service")
	bridge.NewBridgeService(cfg)
}

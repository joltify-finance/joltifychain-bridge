package main

import (
	"fmt"
	_ "net/http/pprof"

	"gitlab.com/oppy-finance/oppy-bridge/version"

	golog "github.com/ipfs/go-log"
	"github.com/oppyfinance/tss/common"
	"github.com/rs/zerolog"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

func main() {
	misc.SetupBech32Prefix()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	cfg := config.DefaultConfig()
	err := golog.SetLogLevel("tss-lib", "INFO")
	if err != nil {
		panic(err)
	}
	common.InitLog("info", true, "oppyBridge_service")
	if cfg.Version {
		fmt.Printf("oppyBridge %v-%v\n", version.VERSION, version.COMMIT)
		return
	}
}

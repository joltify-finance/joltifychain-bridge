package main

import (
	"fmt"
	"gitlab.com/joltify/joltifychain-bridge/bridge"
	"gitlab.com/joltify/joltifychain-bridge/version"

	golog "github.com/ipfs/go-log"
	"github.com/joltify-finance/tss/common"
	"github.com/rs/zerolog"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func main() {
	misc.SetupBech32Prefix()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	cfg := config.DefaultConfig()
	if cfg.Version {
		fmt.Printf("bridge service %v-%v\n", version.VERSION, version.COMMIT)
		return
	}
	err := golog.SetLogLevel("tss-lib", "INFO")
	if err != nil {
		panic(err)
	}
	common.InitLog("info", true, "bridge_service")
	bridge.NewBridgeService(cfg)
}

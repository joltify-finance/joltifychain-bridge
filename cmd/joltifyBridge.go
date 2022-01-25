package main

import (
	golog "github.com/ipfs/go-log"
	"github.com/joltgeorge/tss/common"
	"github.com/rs/zerolog"
	"gitlab.com/joltify/joltifychain-bridge/bridge"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func main() {
	misc.SetupBech32Prefix()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	config := config.DefaultConfig()
	err := golog.SetLogLevel("tss-lib", "INFO")
	if err != nil {
		panic(err)
	}
	common.InitLog("info", true, "joltifyBridge_service")
	bridge.NewBridgeService(config)
}

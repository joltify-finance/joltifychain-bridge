package main

import (
	golog "github.com/ipfs/go-log"
	"github.com/joltgeorge/tss/common"
	"github.com/rs/zerolog"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/bridge"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain/joltifychain-bridge/misc"
)

func main() {
	misc.SetupBech32Prefix()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	config := config.DefaultConfig()

	_ = golog.SetLogLevel("tss-lib", "INFO")
	common.InitLog("info", true, "joltifyBridge_service")
	bridge.NewBridgeService(config)
}

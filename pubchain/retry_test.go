package pubchain

import (
	"context"
	"sync"
	"testing"
	"time"

	"gitlab.com/joltify/joltifychain-bridge/config"

	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

type TestRetrySuite struct {
	suite.Suite
	pubChain *Instance
}

func (tn *TestRetrySuite) SetupSuite() {
	misc.SetupBech32Prefix()
	wg := sync.WaitGroup{}
	cfg := config.Config{}
	cfg.PubChainConfig.WsAddressBSC = misc.WebsocketTest
	cfg.PubChainConfig.WsAddressETH = misc.WebsocketTest

	bscChainClient, err := NewErc20ChainInfo(cfg.PubChainConfig.WsAddressBSC, "BSC", &wg)
	tn.Require().NoError(err)

	ethChainClient, err := NewErc20ChainInfo(cfg.PubChainConfig.WsAddressETH, "ETH", &wg)
	tn.Require().NoError(err)

	pubChain := &Instance{
		wg:         &wg,
		BSCChain:   bscChainClient,
		EthChain:   ethChainClient,
		poolLocker: &sync.RWMutex{},
	}

	tn.pubChain = pubChain
}

func (tn TestRetrySuite) TestRetry() {
	ctx, cancel := context.WithCancel(context.Background())
	err := tn.pubChain.BSCChain.StartSubscription(ctx, tn.pubChain.wg)
	tn.Require().NoError(err)
	b := <-tn.pubChain.BSCChain.SubChannelNow
	currentBlock := b.Number.Uint64()

	time.Sleep(time.Second * 10)

	err = tn.pubChain.BSCChain.RetryPubChain()
	tn.Require().NoError(err)
	blockInNew := currentBlock + uint64(len(tn.pubChain.ChannelQueue))
	latest := <-tn.pubChain.BSCChain.SubChannelNow
	tn.Require().Equal(blockInNew+1, latest.Number.Uint64())
	cancel()
	tn.pubChain.wg.Wait()
}

func TestRetryEvent(t *testing.T) {
	suite.Run(t, new(TestRetrySuite))
}

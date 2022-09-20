package pubchain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

type TestRetrySuite struct {
	suite.Suite
	pubChain *Instance
}

func (tn *TestRetrySuite) SetupSuite() {
	misc.SetupBech32Prefix()

	wg := sync.WaitGroup{}
	pubChain, err := NewChainInstance(misc.WebsocketTest, nil, nil, &wg)
	if err != nil {
		panic(err)
	}
	tn.pubChain = pubChain
}

func (tn TestRetrySuite) TestRetry() {
	ctx, cancel := context.WithCancel(context.Background())
	err := tn.pubChain.StartSubscription(ctx, tn.pubChain.wg)
	tn.Require().NoError(err)
	b := <-tn.pubChain.SubChannelNow
	currentBlock := b.Number.Uint64()

	time.Sleep(time.Second * 10)

	err = tn.pubChain.RetryPubChain()
	tn.Require().NoError(err)
	blockInNew := currentBlock + uint64(len(tn.pubChain.ChannelQueue))
	latest := <-tn.pubChain.SubChannelNow
	tn.Require().Equal(blockInNew+1, latest.Number.Uint64())
	cancel()
	tn.pubChain.wg.Wait()
}

func TestRetryEvent(t *testing.T) {
	suite.Run(t, new(TestRetrySuite))
}

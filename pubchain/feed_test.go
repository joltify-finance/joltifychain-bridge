package pubchain

import (
	"math/big"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/core/types"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/tokenlist"
	"gotest.tools/assert"
)

const (
	AddrPUSD = "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"
)

func TestFeedTx(t *testing.T) {
	misc.SetupBech32Prefix()
	acc, err := generateRandomPrivKey(3)
	assert.NilError(t, err)
	testTx := types.MustSignNewTx(testKey, types.LatestSigner(genesis.Config), &types.LegacyTx{
		Nonce:    0,
		Value:    big.NewInt(12),
		Gas:      53001,
		GasPrice: big.NewInt(1),
	})

	backend, _ := newTestBackend(t, []*ethTypes.Transaction{testTx})
	defer backend.Close()
	client, err := backend.Attach()
	assert.NilError(t, err)
	defer client.Close()

	assert.NilError(t, err)
	tssServer := TssMock{acc[0].sk}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"}, []string{"BSC"})
	assert.NilError(t, err)
	wg := sync.WaitGroup{}
	mockRetryMap := sync.Map{}
	pi, err := NewChainInstance(misc.WebsocketTest, misc.WebsocketTest, &tssServer, tl, &wg, &mockRetryMap)
	assert.NilError(t, err)

	bscChainClient, err := NewChainInfo(misc.WebsocketTest, "BSC", &wg)
	assert.NilError(t, err)
	pi.BSCChain = bscChainClient

	assert.NilError(t, err)
	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "225",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: "joltpub1addwnpepqgknlvjpa7237gnrm2kakjd37xagm7435hmk6zqf5248dnext9cfshhe990",
		},
	}
	toAddr, err := misc.PoolPubKeyToEthAddress("joltpub1addwnpepqgknlvjpa7237gnrm2kakjd37xagm7435hmk6zqf5248dnext9cfshhe990")
	assert.NilError(t, err)

	a1 := common.NewOutboundReq("test1", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), AddrPUSD, 125, nil, "BSC", true)
	a2 := common.NewOutboundReq("test2", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), AddrPUSD, 125, nil, "BSC", true)
	a3 := common.NewOutboundReq("test3", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), AddrPUSD, 125, nil, "BSC", true)
	a4 := common.NewOutboundReq("test4", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), AddrPUSD, 125, nil, "BSC", true)
	testOutBoundReqs := []*common.OutBoundReq{&a1, &a2, &a3, &a4}

	err = pi.FeedTx(&poolInfo, testOutBoundReqs, "BSC")
	assert.NilError(t, err)
	assert.Equal(t, testOutBoundReqs[0].BlockHeight, int64(125))
	wg.Wait()
}

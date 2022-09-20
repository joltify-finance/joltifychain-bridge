package pubchain

import (
	"math/big"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/core/types"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
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
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	assert.NilError(t, err)
	wg := sync.WaitGroup{}
	pi, err := NewChainInstance(misc.WebsocketTest, &tssServer, tl, &wg)
	assert.NilError(t, err)
	c := ethclient.NewClient(client)
	pi.EthClient = c
	assert.NilError(t, err)
	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "225",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: "oppypub1addwnpepqgknlvjpa7237gnrm2kakjd37xagm7435hmk6zqf5248dnext9cfse7dze7",
		},
	}
	toAddr, err := misc.PoolPubKeyToEthAddress("oppypub1addwnpepqgknlvjpa7237gnrm2kakjd37xagm7435hmk6zqf5248dnext9cfse7dze7")
	assert.NilError(t, err)

	a1 := common.NewOutboundReq("test1", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), AddrPUSD, 125, nil, nil)
	a2 := common.NewOutboundReq("test2", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), AddrPUSD, 125, nil, nil)
	a3 := common.NewOutboundReq("test3", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), AddrPUSD, 125, nil, nil)
	a4 := common.NewOutboundReq("test4", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), AddrPUSD, 125, nil, nil)
	testOutBoundReqs := []*common.OutBoundReq{&a1, &a2, &a3, &a4}

	err = pi.FeedTx(&poolInfo, testOutBoundReqs)
	assert.NilError(t, err)
	assert.Equal(t, testOutBoundReqs[0].BlockHeight, int64(125))
	wg.Wait()
}

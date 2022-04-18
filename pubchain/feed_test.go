package pubchain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/core/types"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
	"gotest.tools/assert"
	"math/big"
	"testing"
)

func TestFeedTx(t *testing.T) {
	misc.SetupBech32Prefix()
	websocketTest := "ws://rpc.joltify.io:8456/"
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
	cfg := config.DefaultConfig()
	pi, err := NewChainInstance(websocketTest, cfg.PubChainConfig.TokenAddress, &tssServer)
	assert.NilError(t, err)
	c := ethclient.NewClient(client)
	pi.EthClient = c
	assert.NilError(t, err)
	testBlockHeight := int64(225)
	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "225",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: "joltpub1addwnpepq2adqkank6j0vwxfcjxzwxkkxqnw4248wu9jmyr957y8l690j5j8ckrsvx2",
		},
	}
	toAddr, err := misc.PoolPubKeyToEthAddress("joltpub1addwnpepq2adqkank6j0vwxfcjxzwxkkxqnw4248wu9jmyr957y8l690j5j8ckrsvx2")
	assert.NilError(t, err)

	a1 := common.NewOutboundReq("test1", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), 125, 120)
	a2 := common.NewOutboundReq("test2", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), 125, 120)
	a3 := common.NewOutboundReq("test3", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), 125, 120)
	a4 := common.NewOutboundReq("test4", acc[0].commAddr, toAddr, sdk.NewCoin("test", sdk.NewInt(1)), 125, 120)
	testOutBoundReqs := []*common.OutBoundReq{&a1, &a2, &a3, &a4}

	err = pi.FeedTx(testBlockHeight, &poolInfo, testOutBoundReqs)
	assert.NilError(t, err)
	assert.Equal(t, testOutBoundReqs[0].BlockHeight, int64(225))
	assert.Equal(t, testOutBoundReqs[0].RoundBlockHeight, int64(225/ROUNDBLOCK))

}

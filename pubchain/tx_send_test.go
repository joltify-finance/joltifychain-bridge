package pubchain

import (
	"context"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
	types2 "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

func generateRandomPrivKey(n int) ([]account, error) {
	randomAccounts := make([]account, n)
	for i := 0; i < n; i++ {
		sk := secp256k1.GenPrivKey()
		pk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, sk.PubKey()) //nolint

		ethAddr, err := misc.PoolPubKeyToEthAddress(pk)
		if err != nil {
			return nil, err
		}
		addrJolt, err := types.AccAddressFromHex(sk.PubKey().Address().String())
		if err != nil {
			return nil, err
		}
		tAccount := account{
			sk,
			pk,
			addrJolt,
			ethAddr,
		}
		randomAccounts[i] = tAccount
	}
	return randomAccounts, nil
}

func TestPubChainInstance_composeTx(t *testing.T) {
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)
	tssServer := TssMock{accs[1].sk}
	pi := PubChainInstance{
		lastTwoPools:       make([]*common.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		tokenAddr:          accs[0].commAddr.String(),
		InboundReqChan:     make(chan *InboundReq, 1),
		tssServer:          &tssServer,
	}

	poolInfo0 := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[0].pk,
			PoolAddr:   accs[0].joltAddr,
		},
	}
	poolInfo1 := vaulttypes.PoolInfo{
		BlockHeight: "101",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[1].pk,
			PoolAddr:   accs[1].joltAddr,
		},
	}

	err = pi.UpdatePool(&poolInfo0)
	require.Nil(t, err)
	err = pi.UpdatePool(&poolInfo1)
	require.Nil(t, err)
	txOption, err := pi.composeTx(accs[1].pk, accs[1].commAddr, big.NewInt(64), 100)
	assert.Nil(t, err)

	tx := types2.NewTx(&types2.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    18,
		To:       &accs[1].commAddr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("test"),
	})

	_, err = txOption.Signer(accs[2].commAddr, tx)
	assert.NotNil(t, err)

	_, err = txOption.Signer(accs[1].commAddr, tx)
	assert.NotNil(t, err)
}

func TestSendToken(t *testing.T) {
	accs, err := generateRandomPrivKey(2)
	assert.Nil(t, err)
	tss := TssMock{
		accs[0].sk,
	}

	websocketTest := "wss://apis-sj.ankr.com/wss/783303b49f7b4f988a67631cc709c8ce/a08ea9fddcad7113ac6454229b82c598/binance/full/test"
	tokenAddrTest := "0x0cD80A18df1C5eAd4B5Fb549391d58B06EFfDBC4"
	pubChain, err := NewChainInstance(websocketTest, tokenAddrTest, &tss)
	assert.Nil(t, err)

	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[0].pk,
			PoolAddr:   accs[0].joltAddr,
		},
	}

	err = pubChain.UpdatePool(&poolInfo)
	require.NoError(t, err)

	// we firstly test the subscription of the pubchain new block
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	sbHead, err := pubChain.StartSubscription(ctx, &wg)
	assert.Nil(t, err)

	counter := 0
	for {
		<-sbHead
		counter += 1
		if counter > 2 {
			cancel()
			break
		}
	}
	cancel()
	wg.Wait()

	// now we test send the token
	_, err = pubChain.ProcessOutBound(accs[0].commAddr, accs[1].commAddr, big.NewInt(100), int64(10))
	pubChain.tssServer.Stop()
	assert.EqualError(t, err, "insufficient funds for gas * price + value")
}

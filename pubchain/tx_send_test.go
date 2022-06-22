package pubchain

import (
	"context"
	"encoding/hex"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
	types2 "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
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
		addrOppy, err := types.AccAddressFromHex(sk.PubKey().Address().String())
		if err != nil {
			return nil, err
		}
		tAccount := account{
			sk,
			pk,
			addrOppy,
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
	pi := Instance{
		lastTwoPools:       make([]*common.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		InboundReqChan:     make(chan *common.InBoundReq, 1),
		tssServer:          &tssServer,
	}

	poolInfo0 := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[0].pk,
			PoolAddr:   accs[0].oppyAddr,
		},
	}
	poolInfo1 := vaulttypes.PoolInfo{
		BlockHeight: "101",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[1].pk,
			PoolAddr:   accs[1].oppyAddr,
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
	acc991 := "64285613d3569bcaa7a24c9d74d4cec5c18dcf6a08e4c0f66596078f3a4a35b5"

	data, err := hex.DecodeString(acc991)
	if err != nil {
		panic(err)
	}
	sk := secp256k1.PrivKey{Key: data}
	pk, err := legacybech32.MarshalPubKey(legacybech32.AccPK, sk.PubKey()) //nolint
	if err != nil {
		panic(err)
	}

	fromAddress, err := misc.PoolPubKeyToOppyAddress(pk)
	if err != nil {
		panic(err)
	}
	fromAddrEth, err := misc.PoolPubKeyToEthAddress(pk)
	assert.Nil(t, err)

	tss := TssMock{
		sk: &sk,
	}
	websocketTest := "ws://rpc.test.oppy.zone:8456/"
	tokenAddrTest := "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"
	tl, err := createMockTokenlist("testAddr", "testDenom")
	assert.Nil(t, err)
	pubChain, err := NewChainInstance(websocketTest, &tss, tl)
	assert.Nil(t, err)

	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: pk,
			PoolAddr:   fromAddress,
		},
	}

	err = pubChain.UpdatePool(&poolInfo)
	assert.NoError(t, err)

	// we firstly test the subscription of the pubchain new block
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	sbHead, err := pubChain.StartSubscription(ctx, &wg)
	assert.Nil(t, err)

	counter := 0
	for {
		<-sbHead
		counter++
		if counter > 2 {
			cancel()
			break
		}
	}
	cancel()
	wg.Wait()

	// now we test send the token with wrong nonce
	wg2 := sync.WaitGroup{}
	nonce, err := pubChain.EthClient.PendingNonceAt(context.Background(), fromAddrEth)
	assert.Nil(t, err)
	_, err = pubChain.ProcessOutBound(&wg2, accs[0].commAddr, fromAddrEth, tokenAddrTest, big.NewInt(100), int64(10), nonce)
	assert.Nil(t, err)
	wg2.Wait()

	// now we test send the token
	nonce, err = pubChain.EthClient.PendingNonceAt(context.Background(), fromAddrEth)
	assert.Nil(t, err)

	_, err = pubChain.ProcessOutBound(&wg2, accs[0].commAddr, fromAddrEth, tokenAddrTest, big.NewInt(100), int64(10), nonce)
	assert.Nil(t, err)

	pubChain.tssServer.Stop()
}

package pubchain

import (
	"bytes"
	"context"
	"encoding/hex"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"math/big"
	"sync"
	"testing"
	"time"

	zlog "github.com/rs/zerolog/log"

	"github.com/stretchr/testify/require"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"
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
		lastTwoPools:   make([]*common.PoolInfo, 2),
		poolLocker:     &sync.RWMutex{},
		InboundReqChan: make(chan []*common.InBoundReq, 1),
		tssServer:      &tssServer,
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

func TestProcessOutBound(t *testing.T) {
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
	tokenAddrTest := "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	assert.Nil(t, err)
	wg := sync.WaitGroup{}
	pubChain, err := NewChainInstance(misc.WebsocketTest, &tss, tl, &wg)
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

	// now we test send the token with wrong nonce
	tssWaitGroup := sync.WaitGroup{}
	needToBeProcessed := 2
	tssReqChan := make(chan *TssReq, needToBeProcessed)
	bc := NewBroadcaster()

	defer close(tssReqChan)
	tssWaitGroup.Add(2)
	nonce, err := pubChain.EthClient.PendingNonceAt(context.Background(), fromAddrEth)
	assert.Nil(t, err)
	v2, ret := new(big.Int).SetString("10000000000000000000000", 10)
	assert.True(t, ret)
	testAmounts := []*big.Int{big.NewInt(100), v2}
	for i := 0; i < 2; i++ {
		go func(index int) {
			defer tssWaitGroup.Done()
			tssRespChan, err := bc.Subscribe(int64(index))
			assert.Nil(t, err)
			defer bc.Unsubscribe(int64(index))
			if index == 0 {
				_, err = pubChain.ProcessOutBound(index, fromAddrEth, fromAddrEth, tokenAddrTest, testAmounts[index], nonce+uint64(index), tssReqChan, tssRespChan)
				assert.Nil(t, err)

			} else {
				_, err = pubChain.ProcessOutBound(index, fromAddrEth, fromAddrEth, "native", testAmounts[index], nonce+uint64(index), tssReqChan, tssRespChan)
				assert.NotNil(t, err)
			}
		}(i)
	}

	tssWaitGroup.Add(1)
	go func() {
		defer tssWaitGroup.Done()
		var allsignMSgs [][]byte
		received := make(map[int][]byte)
		collected := false
		for {
			msg := <-tssReqChan
			received[msg.Index] = msg.Data
			if len(received) >= needToBeProcessed {
				collected = true
			}
			if collected {
				break
			}
		}
		for _, val := range received {
			if bytes.Equal([]byte("none"), val) {
				continue
			}
			allsignMSgs = append(allsignMSgs, val)
		}

		lastPool := pubChain.GetPool()[1]
		latest, err := pubChain.GetBlockByNumberWithLock(nil)
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to get the latest height")
			bc.Broadcast(nil)
			return
		}

		blockHeight := int64(latest.NumberU64()) / ROUNDBLOCK
		signature, err := pubChain.TssSignBatch(allsignMSgs, lastPool.Pk, blockHeight)
		if err != nil {
			zlog.Info().Msgf("fail to run batch keysign")
		}
		bc.Broadcast(signature)
	}()
	tssWaitGroup.Wait()

	pubChain.tssServer.Stop()
}

func TestSendToken(t *testing.T) {

	justForTest := "64285613d3569bcaa7a24c9d74d4cec5c18dcf6a08e4c0f66596078f3a4a35b5"

	data, err := hex.DecodeString(justForTest)
	if err != nil {
		panic(err)
	}
	sk := secp256k1.PrivKey{Key: data}
	pk, err := legacybech32.MarshalPubKey(legacybech32.AccPK, sk.PubKey()) //nolint
	if err != nil {
		panic(err)
	}

	tss := TssMock{
		sk: &sk,
	}
	tokenAddrTest := "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	assert.Nil(t, err)
	wg := sync.WaitGroup{}
	pubChain, err := NewChainInstance(misc.WebsocketTest, &tss, tl, &wg)
	assert.Nil(t, err)

	// incorrect address
	receiver := ethcommon.BytesToAddress(sk.PubKey().Address().Bytes())
	_, err = pubChain.SendToken(pk, receiver, receiver, big.NewInt(100), nil, tokenAddrTest)
	assert.Error(t, err)

	// we send token to myself for testing
	receiver, err = misc.PoolPubKeyToEthAddress(pk)
	assert.Nil(t, err)
	jusdAddr := "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"
	_, err = pubChain.SendToken(pk, receiver, receiver, big.NewInt(100), nil, jusdAddr)
	assert.NoError(t, err)
}

func TestSendNativeToken(t *testing.T) {
	justForTest := "64285613d3569bcaa7a24c9d74d4cec5c18dcf6a08e4c0f66596078f3a4a35b5"
	data, err := hex.DecodeString(justForTest)
	if err != nil {
		panic(err)
	}
	sk := secp256k1.PrivKey{Key: data}
	pk, err := legacybech32.MarshalPubKey(legacybech32.AccPK, sk.PubKey()) //nolint
	if err != nil {
		panic(err)
	}

	tss := TssMock{
		sk: &sk,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	assert.Nil(t, err)
	wg := sync.WaitGroup{}
	pubChain, err := NewChainInstance(misc.WebsocketTest, &tss, tl, &wg)
	assert.Nil(t, err)

	// incorrect address,thus it is clear to move fund
	receiver := ethcommon.BytesToAddress(sk.PubKey().Address().Bytes())
	_, ret, err := pubChain.SendNativeTokenForMoveFund(pk, receiver, receiver, big.NewInt(100), big.NewInt(1))
	assert.True(t, ret)

	//assert.Error(t, err)

	// we send token to myself for testing
	receiver, err = misc.PoolPubKeyToEthAddress(pk)
	assert.NoError(t, err)
	// not enough amount pay the gas, returned
	_, ret, err = pubChain.SendNativeTokenForMoveFund(pk, receiver, receiver, big.NewInt(100), big.NewInt(1))
	assert.True(t, ret)
	assert.NoError(t, err)

	// we move the bnb
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	nonce, err := pubChain.getPendingNonceWithLock(ctx, receiver)
	assert.NoError(t, err)
	_, ret, err = pubChain.SendNativeTokenForMoveFund(pk, receiver, receiver, big.NewInt(7830000000000000), big.NewInt(int64(nonce)))
	assert.False(t, ret)
	assert.NoError(t, err)
}

func TestSendNativeTokenBatch(t *testing.T) {

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

	fromAddrEth, err := misc.PoolPubKeyToEthAddress(pk)
	assert.Nil(t, err)

	tss := TssMock{
		sk: &sk,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	assert.Nil(t, err)
	wg := sync.WaitGroup{}
	pubChain, err := NewChainInstance(misc.WebsocketTest, &tss, tl, &wg)
	assert.Nil(t, err)

	// now we test send the token with wrong nonce
	tssWaitGroup := sync.WaitGroup{}
	needToBeProcessed := 2
	tssReqChan := make(chan *TssReq, needToBeProcessed)
	bc := NewBroadcaster()

	defer close(tssReqChan)
	tssWaitGroup.Add(2)
	nonce, err := pubChain.EthClient.PendingNonceAt(context.Background(), fromAddrEth)
	assert.Nil(t, err)
	v2, ok := new(big.Int).SetString("2283000000000000000000", 10)
	assert.True(t, ok)
	amounts := []*big.Int{big.NewInt(22830000000000000), v2}
	for i := 0; i < 2; i++ {
		go func(index int) {
			defer tssWaitGroup.Done()
			tssRespChan, err := bc.Subscribe(int64(index))
			assert.Nil(t, err)
			defer bc.Unsubscribe(int64(index))
			indexSend := int64(nonce) + int64(index)
			_, _, err = pubChain.SendNativeTokenBatch(index, fromAddrEth, fromAddrEth, amounts[index], big.NewInt(indexSend), tssReqChan, tssRespChan)
			if index == 0 {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		}(i)
	}

	tssWaitGroup.Add(1)
	go func() {
		defer tssWaitGroup.Done()
		var allsignMSgs [][]byte
		received := make(map[int][]byte)
		collected := false
		for {
			msg := <-tssReqChan
			received[msg.Index] = msg.Data
			if len(received) >= needToBeProcessed {
				collected = true
			}
			if collected {
				break
			}
		}
		for _, val := range received {
			if bytes.Equal([]byte("none"), val) {
				continue
			}
			allsignMSgs = append(allsignMSgs, val)
		}

		latest, err := pubChain.GetBlockByNumberWithLock(nil)
		if err != nil {
			zlog.Logger.Error().Err(err).Msgf("fail to get the latest height")
			bc.Broadcast(nil)
			return
		}

		blockHeight := int64(latest.NumberU64()) / ROUNDBLOCK
		signature, err := pubChain.TssSignBatch(allsignMSgs, pk, blockHeight)
		if err != nil {
			zlog.Info().Msgf("fail to run batch keysign")
		}
		bc.Broadcast(signature)
	}()
	tssWaitGroup.Wait()

	pubChain.tssServer.Stop()
}

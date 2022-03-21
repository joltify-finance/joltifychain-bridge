package pubchain

import (
	"encoding/hex"
	"fmt"
	"hash"
	"math/big"
	"strings"
	"sync"
	"testing"

	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	common2 "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/generated"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"golang.org/x/crypto/sha3"
)

type account struct {
	sk       *secp256k1.PrivKey
	pk       string
	joltAddr sdk.AccAddress
	commAddr common.Address
}

func TestUpdatePoolAndGetPool(t *testing.T) {
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)

	ci := PubChainInstance{
		lastTwoPools: make([]*common2.PoolInfo, 2),
		poolLocker:   &sync.RWMutex{},
	}
	poolInfo := vaulttypes.PoolInfo{
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
	err = ci.UpdatePool(nil)
	assert.NotNil(t, err)
	err = ci.UpdatePool(&poolInfo)
	require.Nil(t, err)
	pools := ci.GetPool()
	require.Equal(t, pools[1].Pk, accs[0].pk)

	err = ci.UpdatePool(&poolInfo1)
	require.Nil(t, err)
	pools = ci.GetPool()
	require.Equal(t, pools[0].EthAddress.Hex(), accs[0].commAddr.Hex())
	require.Equal(t, pools[1].EthAddress.Hex(), accs[1].commAddr.Hex())

	poolInfo2 := vaulttypes.PoolInfo{
		BlockHeight: "102",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[2].pk,
			PoolAddr:   accs[2].joltAddr,
		},
	}
	//
	err = ci.UpdatePool(&poolInfo2)
	require.Nil(t, err)
	pools = ci.GetPool()
	require.Equal(t, pools[0].EthAddress.Hex(), accs[1].commAddr.Hex())
	require.Equal(t, pools[1].EthAddress.Hex(), accs[2].commAddr.Hex())
}

func TestCheckToBridge(t *testing.T) {
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)

	ci := PubChainInstance{
		lastTwoPools: make([]*common2.PoolInfo, 2),
		poolLocker:   &sync.RWMutex{},
	}

	poolInfo := vaulttypes.PoolInfo{
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

	poolInfo2 := vaulttypes.PoolInfo{
		BlockHeight: "102",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[2].pk,
			PoolAddr:   accs[2].joltAddr,
		},
	}
	err = ci.UpdatePool(&poolInfo)
	require.Nil(t, err)

	ret := ci.checkToBridge(accs[1].commAddr)
	require.False(t, ret)

	err = ci.UpdatePool(&poolInfo1)
	require.Nil(t, err)
	ret = ci.checkToBridge(accs[1].commAddr)
	require.True(t, ret)

	err = ci.UpdatePool(&poolInfo2)
	require.Nil(t, err)

	ret = ci.checkToBridge(accs[2].commAddr)
	require.True(t, ret)

	ret = ci.checkToBridge(accs[1].commAddr)
	require.True(t, ret)

	ret = ci.checkToBridge(accs[0].commAddr)
	require.False(t, ret)
}

func TestProcessInBound(t *testing.T) {
	toStr := "FFcf8FDEE72ac11b5c542428B35EEF5769C409f0"
	privkey991 := "64285613d3569bcaa7a24c9d74d4cec5c18dcf6a08e4c0f66596078f3a4a35b5"
	privateKey, err := crypto.HexToECDSA(privkey991)
	assert.Nil(t, err)
	pi := PubChainInstance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
	}

	testAmount := big.NewInt(100)
	signer := ethTypes.NewEIP155Signer(big.NewInt(18))

	addr := crypto.PubkeyToAddress(privateKey.PublicKey)
	tx, err := ethTypes.SignTx(ethTypes.NewTransaction(0, addr, new(big.Int), 0, new(big.Int), nil), signer, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	transferTo := common.HexToAddress(toStr)
	tokenAddrStr := "0x33875278f7757f6b43abC223EeC4a9D6204186a0"
	tokenAddr := common.HexToAddress(tokenAddrStr)

	err = pi.ProcessInBoundERC20(tx, tokenAddr, transferTo, testAmount, uint64(10))
	require.NotNil(t, err)
	require.EqualError(t, err, "incorrect top up token")

	pi.tokenAddr = tokenAddrStr
	err = pi.ProcessInBoundERC20(tx, tokenAddr, transferTo, testAmount, uint64(10))
	require.Nil(t, err)

	err = pi.ProcessInBoundERC20(tx, tokenAddr, transferTo, testAmount, uint64(10))
	fmt.Printf(">>>>>>%v\n", err)
	require.EqualError(t, err, "tx existed")
}

func TestUpdateBridgeTx(t *testing.T) {
	pi := PubChainInstance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
	}
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)

	coin := sdk.Coin{
		Denom:  config.InBoundDenom,
		Amount: sdk.NewIntFromUint64(32),
	}

	feeCoin := sdk.Coin{
		Denom:  config.InBoundDenomFee,
		Amount: sdk.NewIntFromUint64(0),
	}
	btx := inboundTx{
		accs[1].joltAddr,
		uint64(10),
		coin,
		feeCoin,
	}
	pi.pendingInbounds.Store("test1", &btx)
	// now we should have successfully top up the token
	ret := pi.updateInboundTx("test1", big.NewInt(10), uint64(11))
	require.Equal(t, ret.address.String(), accs[1].joltAddr.String())
	// now we top up the tx that not exist, and we should store this tx in pending bnb pool
	ret = pi.updateInboundTx("test2", big.NewInt(20), uint64(29))
	require.Nil(t, ret)
	_, exist := pi.pendingInboundsBnB.Load("test2")
	require.True(t, exist)

	// now we process the tx with two top-up
	coin = sdk.Coin{
		Denom:  config.InBoundDenom,
		Amount: sdk.NewIntFromUint64(32),
	}

	feeCoin = sdk.Coin{
		Denom:  config.InBoundDenomFee,
		Amount: sdk.NewIntFromUint64(0),
	}
	btx = inboundTx{
		accs[1].joltAddr,
		uint64(10),
		coin,
		feeCoin,
	}
	pi.pendingInbounds.Store("test2", &btx)

	ret = pi.updateInboundTx("test2", big.NewInt(1), uint64(32))
	require.Nil(t, ret)
	//
	//// if we do not have enough fee paid
	ret = pi.updateInboundTx("test2", big.NewInt(8), uint64(33))
	require.Nil(t, ret)

	ret = pi.updateInboundTx("test2", big.NewInt(1), uint64(34))
	require.Equal(t, ret.address.String(), accs[1].joltAddr.String())
}

type testHasher struct {
	hasher hash.Hash
}

func (h *testHasher) Reset() {
	h.hasher.Reset()
}

func (h *testHasher) Update(key, val []byte) {
	h.hasher.Write(key)
	h.hasher.Write(val)
}

func (h *testHasher) Hash() common.Hash {
	return common.BytesToHash(h.hasher.Sum(nil))
}

func newHasher() *testHasher {
	return &testHasher{hasher: sha3.NewLegacyKeccak256()}
}

func TestProcessEachBlock(t *testing.T) {
	misc.SetupBech32Prefix()
	accs, err := generateRandomPrivKey(2)
	assert.Nil(t, err)
	emptyEip2718Tx := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &accs[0].commAddr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("wrongTx"),
	})

	emptyEip2718TxNotToBridge := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &accs[1].commAddr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("testerTx"),
	})

	// fee, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	emptyEip2718TxGoodTopUpFee := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &accs[0].commAddr,
		Value:    big.NewInt(8),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("test1"),
	})

	// fee, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	emptyEip2718TxGoodTopUpEmptyData := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &accs[0].commAddr,
		Value:    big.NewInt(8),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     nil,
	})

	// fee, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	emptyEip2718TxGoodTopUpFeeBeforeERC20 := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &accs[0].commAddr,
		Value:    big.NewInt(8),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("ERC20NOTREADY"),
	})

	tBlock := ethTypes.Block{}

	tAbi, err := abi.JSON(strings.NewReader(generated.TokenMetaData.ABI))
	assert.Nil(t, err)
	pi := PubChainInstance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		tokenAbi:           &tAbi,
		RetryInboundReq:    &sync.Map{},
		InboundReqChan:     make(chan *InboundReq, 1),
	}

	coin := sdk.Coin{
		Denom:  config.InBoundDenom,
		Amount: sdk.NewIntFromUint64(32),
	}

	feeCoin := sdk.Coin{
		Denom:  config.InBoundDenomFee,
		Amount: sdk.NewIntFromUint64(1),
	}

	btx := inboundTx{
		accs[0].joltAddr,
		uint64(10),
		coin,
		feeCoin,
	}
	pi.pendingInbounds.Store(hex.EncodeToString([]byte("test1")), &btx)

	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[0].pk,
			PoolAddr:   accs[0].joltAddr,
		},
	}

	err = pi.UpdatePool(&poolInfo)
	require.Nil(t, err)
	pi.processEachBlock(&tBlock, 10)
	ret, exist := pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	// indicate nothing happens
	require.True(t, exist)
	storedInbound := ret.(*inboundTx)
	require.Equal(t, storedInbound.address.String(), accs[0].joltAddr.String())

	header := &ethTypes.Header{
		Difficulty: math.BigPow(11, 11),
		Number:     math.BigPow(2, 9),
		GasLimit:   12345678,
		GasUsed:    1476322,
		Time:       9876543,
		Extra:      []byte("coolest block on pub_chain"),
	}
	//
	tBlock1 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718Tx}, nil, nil, newHasher())
	pi.processEachBlock(tBlock1, 10)
	ret, exist = pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	require.True(t, exist)
	storedInbound = ret.(*inboundTx)
	require.Equal(t, storedInbound.address.String(), accs[0].joltAddr.String())

	// check not to bridge
	tBlock2 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxNotToBridge}, nil, nil, newHasher())
	pi.processEachBlock(tBlock2, 10)
	ret, exist = pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	require.True(t, exist)
	storedInbound = ret.(*inboundTx)
	require.Equal(t, storedInbound.fee.Amount.String(), feeCoin.Amount.String())

	//
	//// now we top up the fee
	tBlock3 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpFee}, nil, nil, newHasher())
	pi.processEachBlock(tBlock3, 10)

	ret, exist = pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	require.True(t, exist)
	storedInbound = ret.(*inboundTx)
	topupFee := sdk.NewIntFromBigInt(emptyEip2718TxGoodTopUpFee.Value())
	require.True(t, storedInbound.fee.Amount.Equal(topupFee.Add(feeCoin.Amount)))

	tBlock3 = ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpEmptyData}, nil, nil, newHasher())
	pi.processEachBlock(tBlock3, 10)

	ret, exist = pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	require.True(t, exist)
	storedInbound = ret.(*inboundTx)
	require.True(t, storedInbound.fee.Amount.Equal(topupFee.Add(feeCoin.Amount)))
	//
	tBlock3 = ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpFee}, nil, nil, newHasher())
	pi.processEachBlock(tBlock3, 10)
	_, exist = pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	require.False(t, exist)

	//
	//// now we top up the fee before ERC20 tx arrive
	tBlock4 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpFeeBeforeERC20}, nil, nil, newHasher())
	pi.processEachBlock(tBlock4, 10)
	ret, ok := pi.pendingInboundsBnB.Load(hex.EncodeToString([]byte("ERC20NOTREADY")))
	assert.True(t, ok)
	data := ret.(*inboundTxBnb)
	assert.Equal(t, data.fee.Amount.String(), emptyEip2718TxGoodTopUpFeeBeforeERC20.Value().String())
}

func TestProcessEachBlockErc20(t *testing.T) {
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)
	tAbi, err := abi.JSON(strings.NewReader(generated.TokenMetaData.ABI))
	assert.Nil(t, err)
	pi := PubChainInstance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		tokenAbi:           &tAbi,
		InboundReqChan:     make(chan *InboundReq, 1),
		tokenAddr:          accs[1].commAddr.String(),
	}

	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[0].pk,
			PoolAddr:   accs[0].joltAddr,
		},
	}

	err = pi.UpdatePool(&poolInfo)
	assert.Nil(t, err)

	header := &ethTypes.Header{
		Difficulty: math.BigPow(11, 11),
		Number:     math.BigPow(2, 9),
		GasLimit:   12345678,
		GasUsed:    1476322,
		Time:       9876543,
		Extra:      []byte("coolest block on pub_chain"),
	}

	method, ok := tAbi.Methods["transfer"]
	assert.True(t, ok)
	dataRaw, err := method.Inputs.Pack(accs[2].commAddr, big.NewInt(10))
	assert.Nil(t, err)
	// we need to put 4 leading 0 to match the format in test
	data := append([]byte("0000"), dataRaw...)

	Eip2718Tx := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &accs[2].commAddr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     data,
	})

	// since token addr is not set, so the system should not put this tx in top-up queue
	tBlock := ethTypes.NewBlock(header, []*ethTypes.Transaction{Eip2718Tx}, nil, nil, newHasher())
	pi.processEachBlock(tBlock, 10)
	counter := 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, counter, 0)

	// though to the token addr but not to the pool, so we ignore this tx
	Eip2718TxNotPool := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &accs[1].commAddr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     data,
	})
	tBlock = ethTypes.NewBlock(header, []*ethTypes.Transaction{Eip2718TxNotPool}, nil, nil, newHasher())
	pi.processEachBlock(tBlock, 10)
	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, counter, 0)

	dataRaw, err = method.Inputs.Pack(accs[0].commAddr, big.NewInt(10))
	assert.Nil(t, err)

	dataToSign := []byte("hello")
	hash := crypto.Keccak256Hash(dataToSign)
	sk, err := crypto.ToECDSA(accs[0].sk.Bytes())
	assert.Nil(t, err)
	signature, err := crypto.Sign(hash.Bytes(), sk)
	assert.Nil(t, err)
	r := signature[:32]
	s := signature[32:64]
	v := signature[64:65]

	// we need to put 4 leading 0 to match the format in test
	data = append([]byte("0000"), dataRaw...)
	// though to the token addr but not to the pool, so we ignore this tx
	Eip2718TxGoodPass := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &accs[1].commAddr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     data,
		R:        new(big.Int).SetBytes(r),
		S:        new(big.Int).SetBytes(s),
		V:        new(big.Int).SetBytes(v),
	})

	tBlock = ethTypes.NewBlock(header, []*ethTypes.Transaction{Eip2718TxGoodPass}, nil, nil, newHasher())
	pi.processEachBlock(tBlock, 10)

	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, counter, 1)
}

func TestDeleteExpire(t *testing.T) {
	acc, err := generateRandomPrivKey(1)
	assert.Nil(t, err)

	coin := sdk.Coin{
		Denom:  "testtoken",
		Amount: sdk.NewInt(3),
	}

	firstBlock := uint64(10)
	secondBlock := uint64(20)
	thirdBlock := uint64(30)
	btx1 := inboundTx{
		acc[0].joltAddr,
		firstBlock,
		coin,
		coin,
	}

	btx2 := inboundTx{
		acc[0].joltAddr,
		secondBlock,
		coin,
		coin,
	}

	btx3 := inboundTx{
		acc[0].joltAddr,
		thirdBlock,
		coin,
		coin,
	}

	pi := PubChainInstance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		InboundReqChan:     make(chan *InboundReq, 1),
	}
	pi.pendingInbounds.Store("test1", &btx1)
	pi.pendingInbounds.Store("test2", &btx2)
	pi.pendingInbounds.Store("test3", &btx3)

	currentBlock := uint64(50)
	pi.DeleteExpired(currentBlock)
	// check we do not need to delete
	counter := 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 3, counter)

	// we only delete the first one
	currentBlock = secondBlock + config.TxTimeout
	pi.DeleteExpired(currentBlock)
	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 2, counter)

	pi.pendingInbounds.Store("test1", &btx1)
	pi.pendingInbounds.Store("test2", &btx2)
	pi.pendingInbounds.Store("test3", &btx3)

	currentBlock = secondBlock + config.TxTimeout + 10
	pi.DeleteExpired(currentBlock)
	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 1, counter)

	pi.pendingInbounds.Store("test1", &btx1)
	pi.pendingInbounds.Store("test2", &btx2)
	pi.pendingInbounds.Store("test3", &btx3)

	currentBlock = thirdBlock + config.TxTimeout + 1
	pi.DeleteExpired(currentBlock)
	// check we do not need to delete
	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 0, counter)
}

func TestDeleteExpireBnB(t *testing.T) {
	coin := sdk.Coin{
		Denom:  "testtoken",
		Amount: sdk.NewInt(3),
	}

	firstBlock := uint64(10)
	secondBlock := uint64(20)
	thirdBlock := uint64(30)
	btx1 := inboundTxBnb{
		firstBlock,
		"bnb1",
		coin,
	}

	btx2 := inboundTxBnb{
		secondBlock,
		"bnb1",
		coin,
	}

	btx3 := inboundTxBnb{
		thirdBlock,
		"bnb1",
		coin,
	}

	pi := PubChainInstance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		InboundReqChan:     make(chan *InboundReq, 1),
	}
	pi.pendingInboundsBnB.Store("test1", &btx1)
	pi.pendingInboundsBnB.Store("test2", &btx2)
	pi.pendingInboundsBnB.Store("test3", &btx3)

	currentBlock := uint64(50)
	pi.DeleteExpired(currentBlock)
	// check we do not need to delete
	counter := 0
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 3, counter)

	// we only delete the first one
	currentBlock = secondBlock + config.TxTimeout
	pi.DeleteExpired(currentBlock)

	counter = 0
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 2, counter)

	pi.pendingInboundsBnB.Store("test1", &btx1)
	pi.pendingInboundsBnB.Store("test2", &btx2)
	pi.pendingInboundsBnB.Store("test3", &btx3)

	currentBlock = secondBlock + config.TxTimeout + 10
	pi.DeleteExpired(currentBlock)
	counter = 0
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 1, counter)

	pi.pendingInboundsBnB.Store("test1", &btx1)
	pi.pendingInboundsBnB.Store("test2", &btx2)
	pi.pendingInboundsBnB.Store("test3", &btx3)

	currentBlock = thirdBlock + config.TxTimeout + 1
	pi.DeleteExpired(currentBlock)
	// check we do not need to delete
	counter = 0
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 0, counter)
}

func TestProcessBlockFeeAhead(t *testing.T) {
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)

	pi := PubChainInstance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		tokenAddr:          accs[0].commAddr.String(),
		InboundReqChan:     make(chan *InboundReq, 1),
	}

	bnbTx := inboundTxBnb{
		uint64(11),
		hex.EncodeToString([]byte("test1")),
		sdk.NewCoin(config.InBoundDenomFee, sdk.NewIntFromUint64(8)),
	}

	pi.pendingInboundsBnB.Store(hex.EncodeToString([]byte("test1")), &bnbTx)
	pi.processInboundTx(hex.EncodeToString([]byte("test1")), uint64(10), accs[1].joltAddr, accs[2].commAddr, big.NewInt(11), accs[0].commAddr)

	counter := 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})

	counter2 := 0
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		counter2 += 1
		return true
	})

	assert.Equal(t, 0, counter2)

	pi.pendingInbounds = &sync.Map{}
	pi.pendingInboundsBnB = &sync.Map{}
	bnbTx2 := inboundTxBnb{
		uint64(11),
		hex.EncodeToString([]byte("test2")),
		sdk.NewCoin(config.InBoundDenomFee, sdk.NewIntFromUint64(18)),
	}

	pi.pendingInboundsBnB.Store(hex.EncodeToString([]byte("test2")), &bnbTx2)
	pi.processInboundTx(hex.EncodeToString([]byte("test2")), uint64(10), accs[1].joltAddr, accs[2].commAddr, big.NewInt(11), accs[0].commAddr)

	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter += 1
		return true
	})
	assert.Equal(t, 0, counter)

	counter2 = 0
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		counter2 += 1
		return true
	})

	assert.Equal(t, 0, counter2)
}

func TestAccountVerify(t *testing.T) {
	misc.SetupBech32Prefix()
	type fields struct {
		address sdk.AccAddress
		token   sdk.Coin
		fee     sdk.Coin
	}

	addr, err := sdk.AccAddressFromBech32("jolt1ljh48799pqcnezpsjr69ukpfq4mgapvpr7kzhm")

	minFee, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	assert.Nil(t, err)
	deltaFee, err := sdk.NewDecFromStr("0.00001")
	assert.Nil(t, err)
	larger := minFee.Add(deltaFee)
	small := minFee.Sub(deltaFee)

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "test ok",
			fields: fields{
				address: addr,
				fee:     sdk.Coin{Denom: config.InBoundDenomFee, Amount: sdk.NewIntFromBigInt(larger.BigInt())},
			},
			wantErr: false,
		},
		{
			name: "not enough fee",
			fields: fields{
				address: addr,
				fee:     sdk.Coin{Denom: config.InBoundDenomFee, Amount: sdk.NewIntFromBigInt(small.BigInt())},
			},
			wantErr: true,
		},
		{
			name: "wrong demon",
			fields: fields{
				address: addr,
				fee:     sdk.Coin{Denom: "wrong", Amount: sdk.NewIntFromBigInt(small.BigInt())},
			},
			wantErr: true,
		},

		{
			name: "exact fee",
			fields: fields{
				address: addr,
				fee:     sdk.Coin{Denom: "wrong", Amount: sdk.NewIntFromBigInt(minFee.BigInt())},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &inboundTx{
				address: tt.fields.address,
				token:   tt.fields.token,
				fee:     tt.fields.fee,
			}
			if err := a.Verify(); (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

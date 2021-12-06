package pubchain

import (
	"encoding/hex"
	"hash"
	"math/big"
	"sync"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

func TestUpdatePoolAndGetPool(t *testing.T) {
	account1 := "90F8bf6A479f320ead074411a4B0e7944Ea8c9C1"
	account2 := "FFcf8FDEE72ac11b5c542428B35EEF5769C409f0"
	account3 := "22d491Bde2303f2f43325b2108D26f1eAbA1e32b"
	ci := PubChainInstance{
		lastTwoPools: make([]string, 2),
		poolLocker:   sync.RWMutex{},
	}
	err := ci.UpdatePool("bad")
	require.Error(t, err)

	err = ci.UpdatePool("")
	require.Error(t, err)

	err = ci.UpdatePool(account1)
	require.Nil(t, err)
	pools := ci.GetPool()
	require.Equal(t, pools[0], "")
	require.Equal(t, pools[1], "0x"+account1)

	err = ci.UpdatePool(account2)
	require.Nil(t, err)
	pools = ci.GetPool()
	require.Equal(t, pools[0], "0x"+account1)
	require.Equal(t, pools[1], "0x"+account2)

	err = ci.UpdatePool(account3)
	require.Nil(t, err)
	pools = ci.GetPool()
	require.Equal(t, pools[0], "0x"+account2)
	require.Equal(t, pools[1], "0x"+account3)
}

func TestCheckToBridge(t *testing.T) {
	account1 := "90F8bf6A479f320ead074411a4B0e7944Ea8c9C1"
	account2 := "FFcf8FDEE72ac11b5c542428B35EEF5769C409f0"
	ci := PubChainInstance{
		lastTwoPools: make([]string, 2),
		poolLocker:   sync.RWMutex{},
	}

	err := ci.UpdatePool(account1)
	require.Nil(t, err)

	ret := ci.checkToBridge("bad")
	require.False(t, ret)

	err = ci.UpdatePool(account2)
	require.Nil(t, err)

	ret = ci.checkToBridge("0x" + account2)
	require.True(t, ret)

	ret = ci.checkToBridge("bad")
	require.False(t, ret)

	err = ci.UpdatePool(account1)
	require.Nil(t, err)

	err = ci.UpdatePool(account1)
	require.Nil(t, err)

	ret = ci.checkToBridge("0x" + account2)
	require.False(t, ret)
}

func TestProcessInBound(t *testing.T) {
	fromStr := "90F8bf6A479f320ead074411a4B0e7944Ea8c9C1"
	toStr := "FFcf8FDEE72ac11b5c542428B35EEF5769C409f0"
	ci := PubChainInstance{
		lastTwoPools:    make([]string, 2),
		poolLocker:      sync.RWMutex{},
		pendingInbounds: make(map[string]*inboundTx),
	}

	log := ethTypes.Log{Removed: true}
	log2 := ethTypes.Log{
		Removed: false,
		Address: common.HexToAddress("bad"),
	}
	testAmount := big.NewInt(100)
	tToken := TokenTransfer{
		common.HexToAddress(fromStr),
		common.HexToAddress(toStr),
		testAmount,
		log,
	}
	err := ci.ProcessInBound(&tToken)
	require.NotNil(t, err)
	require.EqualError(t, err, "the tx is the revert tx")

	tToken.Raw = log2
	err = ci.ProcessInBound(&tToken)
	require.NotNil(t, err)
	require.EqualError(t, err, "incorrect top up token")

	tToken.Raw.Address = common.HexToAddress("0xtestAddr")
	testCoin := sdk.Coin{
		Denom:  iNBoundToken,
		Amount: sdk.NewIntFromBigInt(testAmount),
	}
	testTxHash := common.BytesToHash([]byte("0xa9ec0ae9060f319611f2155ccab03200d1fb4a0cb7635f91c04df1521825401a"))
	tToken.Raw.TxHash = testTxHash
	err = ci.ProcessInBound(&tToken)
	require.EqualError(t, err, "incorrect top up token")
	tToken.Raw.Address = common.HexToAddress(iNBoundToken)
	err = ci.ProcessInBound(&tToken)
	require.Nil(t, err)
	a := ci.pendingInbounds[testTxHash.Hex()[2:]]
	require.True(t, a.token.Equal(testCoin))

	// same tx hash should be rejected
	err = ci.ProcessInBound(&tToken)
	require.EqualError(t, err, "tx existed")
}

func TestUpdateBridgeTx(t *testing.T) {
	ci := PubChainInstance{
		lastTwoPools:           make([]string, 2),
		poolLocker:             sync.RWMutex{},
		pendingInbounds:        make(map[string]*inboundTx),
		pendingInboundTxLocker: sync.RWMutex{},
	}

	fromStr := "90F8bf6A479f320ead074411a4B0e7944Ea8c9C1"
	coin := sdk.Coin{
		Denom:  iNBoundToken,
		Amount: sdk.NewIntFromUint64(32),
	}

	feeCoin := sdk.Coin{
		Denom:  inBoundDenom,
		Amount: sdk.NewIntFromUint64(0),
	}
	btx := inboundTx{
		fromStr,
		inBound,
		time.Now(),
		coin,
		feeCoin,
	}
	ci.pendingInbounds["tester"] = &btx

	ret := ci.updateInboundTx("false", big.NewInt(1), inBound)
	require.Nil(t, ret)

	ret = ci.updateInboundTx("tester", big.NewInt(1), outBound)
	require.Nil(t, ret)

	// if we do not have enough fee paid
	ret = ci.updateInboundTx("tester", big.NewInt(123), inBound)
	require.Nil(t, ret)
	require.True(t, ci.pendingInbounds["tester"].fee.Amount.Equal(sdk.NewInt(123)))

	// now we top up the money
	topUp, err := sdk.NewDecFromStr(inBoundFeeMin)
	require.Nil(t, err)

	ret = ci.updateInboundTx("tester", topUp.BigInt(), inBound)
	require.NotNil(t, ret)

	target := sdk.NewInt(123).Add(sdk.NewIntFromBigInt(topUp.BigInt()))
	require.True(t, ret.fee.Amount.Equal(target))

	_, ok := ci.pendingInbounds["tester"]
	require.False(t, ok)
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
	account1 := "90F8bf6A479f320ead074411a4B0e7944Ea8c9C1"
	account2 := "22d491Bde2303f2f43325b2108D26f1eAbA1e32b"
	encodeStr := hex.EncodeToString([]byte("testerTx"))
	addr := common.HexToAddress(account1)
	emptyEip2718Tx := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &addr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("wrongTx"),
	})

	emptyEip2718TxGood := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &addr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("testerTx"),
	})

	fee, err := sdk.NewDecFromStr(inBoundFeeMin)
	require.Nil(t, err)
	emptyEip2718TxGoodTopUpFee := ethTypes.NewTx(&ethTypes.AccessListTx{
		ChainID:  big.NewInt(1),
		Nonce:    3,
		To:       &addr,
		Value:    fee.BigInt(),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("testerTx"),
	})

	tBlock := ethTypes.Block{}

	ci := PubChainInstance{
		lastTwoPools:           make([]common.Address, 2),
		poolLocker:             sync.RWMutex{},
		pendingInbounds:        make(map[string]*inboundTx),
		pendingInboundTxLocker: sync.RWMutex{},
		InboundReqChan:         make(chan *InboundReq, 1),
	}
	fromStr := "90F8bf6A479f320ead074411a4B0e7944Ea8c9C1"
	coin := sdk.Coin{
		Denom:  iNBoundToken,
		Amount: sdk.NewIntFromUint64(32),
	}

	feeCoin := sdk.Coin{
		Denom:  inBoundDenom,
		Amount: sdk.NewIntFromUint64(1),
	}

	btx := inboundTx{
		fromStr,
		inBound,
		time.Now(),
		coin,
		feeCoin,
	}
	ci.pendingInbounds[encodeStr] = &btx
	acc2 := common.HexToAddress(account2)
	err = ci.UpdatePool(acc2)
	require.Nil(t, err)
	ci.processEachBlock(&tBlock)
	ret := ci.pendingInbounds[encodeStr]
	// indicate nothing happens
	require.True(t, ret.fee.Amount.Equal(sdk.NewIntFromUint64(1)))

	header := &ethTypes.Header{
		Difficulty: math.BigPow(11, 11),
		Number:     math.BigPow(2, 9),
		GasLimit:   12345678,
		GasUsed:    1476322,
		Time:       9876543,
		Extra:      []byte("coolest block on pub_chain"),
	}

	tBlock1 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718Tx}, nil, nil, newHasher())
	ci.processEachBlock(tBlock1)
	require.True(t, ret.fee.Amount.Equal(sdk.NewIntFromUint64(1)))

	// now we update it should fail as the txid is incorrect
	err = ci.UpdatePool(account1)
	require.Nil(t, err)

	tBlock2 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718Tx}, nil, nil, newHasher())
	ci.processEachBlock(tBlock2)
	require.True(t, ret.fee.Amount.Equal(sdk.NewIntFromUint64(1)))

	// now we top up the fee
	tBlock3 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGood}, nil, nil, newHasher())
	ci.processEachBlock(tBlock3)
	_, ok := ci.pendingInbounds[encodeStr]
	// the tx should still be in map
	require.True(t, ok)
	require.True(t, ret.fee.Amount.Equal(sdk.NewIntFromUint64(11)))

	// now we top up the fee
	tBlock4 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpFee}, nil, nil, newHasher())
	ci.processEachBlock(tBlock4)
	_, ok = ci.pendingInbounds[encodeStr]
	// the tx should still be in map
	require.False(t, ok)
	totalFee := sdk.NewIntFromUint64(11).Add(sdk.NewIntFromBigInt(fee.BigInt()))
	require.True(t, ret.fee.Amount.Equal(totalFee))
}

package pubchain

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io/ioutil"
	"math/big"
	"os"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"

	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	common2 "gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/generated"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"
	"golang.org/x/crypto/sha3"
)

type account struct {
	sk       *secp256k1.PrivKey
	pk       string
	oppyAddr sdk.AccAddress
	commAddr common.Address
}

func createMockTokenlist(tokenAddr string, tokenDenom string) (*tokenlist.TokenList, error) {
	// init the token list file path
	current, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	// write the temp file for mockTokenlist
	tokenlistPath := path.Join(current, "../test_data/tokenlist/tokenlist_mock.json")
	tempTL := map[string]string{}
	tempTL[tokenAddr] = tokenDenom
	buf, err := json.Marshal(&tempTL)
	if err != nil {
		return nil, err
	}
	err = ioutil.WriteFile(tokenlistPath, buf, 0o655)
	if err != nil {
		return nil, err
	}
	// init the tokenlist with provided tokenAddress,tokenDenom pair
	tl, err := tokenlist.NewTokenList(tokenlistPath, 100)
	if err != nil {
		return nil, err
	}
	return tl, nil
}

func TestUpdatePoolAndGetPool(t *testing.T) {
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)

	ci := Instance{
		lastTwoPools: make([]*common2.PoolInfo, 2),
		poolLocker:   &sync.RWMutex{},
	}
	poolInfo := vaulttypes.PoolInfo{
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
			PoolAddr:   accs[2].oppyAddr,
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

	ci := Instance{
		lastTwoPools: make([]*common2.PoolInfo, 2),
		poolLocker:   &sync.RWMutex{},
	}

	poolInfo := vaulttypes.PoolInfo{
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

	poolInfo2 := vaulttypes.PoolInfo{
		BlockHeight: "102",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[2].pk,
			PoolAddr:   accs[2].oppyAddr,
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
	websocketTest := "ws://rpc.test.oppy.zone:8456/"
	acc, err := generateRandomPrivKey(3)
	assert.Nil(t, err)
	tssServer := TssMock{acc[0].sk}
	tl, err := createMockTokenlist("testDenom", "testAddr")
	assert.Nil(t, err)
	pi, err := NewChainInstance(websocketTest, &tssServer, tl)
	assert.Nil(t, err)

	toStr := "FFcf8FDEE72ac11b5c542428B35EEF5769C409f0"
	privkey991 := "64285613d3569bcaa7a24c9d74d4cec5c18dcf6a08e4c0f66596078f3a4a35b5"
	privateKey, err := crypto.HexToECDSA(privkey991)
	assert.Nil(t, err)

	testAmount := big.NewInt(100)
	signer := ethTypes.NewEIP155Signer(big.NewInt(18))

	addr := crypto.PubkeyToAddress(privateKey.PublicKey)
	tx, err := ethTypes.SignTx(ethTypes.NewTransaction(0, addr, new(big.Int), 0, new(big.Int), nil), signer, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	transferTo := common.HexToAddress(toStr)
	nonExistedTokenAddrStr := "0xthisisanonexitedtoken1234567890123456789"
	nonExistedTokenAddr := common.HexToAddress(nonExistedTokenAddrStr)

	err = pi.ProcessInBoundERC20(tx, nonExistedTokenAddr, transferTo, testAmount, uint64(10))
	require.NotNil(t, err)
	require.EqualError(t, err, "incorrect top up token")

	ExistedTokenAddrStr := "0x15fb343d82cD1C22542261dF408dA8396A829F6B"
	ExistedTokenAddr := common.HexToAddress(ExistedTokenAddrStr)
	pi.TokenList, err = createMockTokenlist(ExistedTokenAddrStr, "testDenom")
	assert.Nil(t, err)

	err = pi.ProcessInBoundERC20(tx, ExistedTokenAddr, transferTo, testAmount, uint64(10))
	require.Nil(t, err)

	err = pi.ProcessInBoundERC20(tx, ExistedTokenAddr, transferTo, testAmount, uint64(10))
	require.EqualError(t, err, "tx existed")
}

func TestUpdateBridgeTx(t *testing.T) {
	pi := Instance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
	}
	InBoundDenomTest1 := "JUSD"
	InBoundDenomTest2 := "JoltBNB"
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)

	coin := sdk.Coin{
		Denom:  InBoundDenomTest1,
		Amount: sdk.NewIntFromUint64(32),
	}

	feeCoin := sdk.Coin{
		Denom:  config.InBoundDenomFee,
		Amount: sdk.NewIntFromUint64(0),
	}
	btx := InboundTx{
		"testTxID",
		accs[1].oppyAddr,
		uint64(10),
		coin,
		feeCoin,
	}
	pi.pendingInbounds.Store("test1", &btx)
	// now we should have successfully top up the token
	ret := pi.updateInboundTx("test1", big.NewInt(10), uint64(11))
	require.Equal(t, ret.Address.String(), accs[1].oppyAddr.String())
	// now we top up the tx that not exist, and we should store this tx in pending bnb pool
	ret = pi.updateInboundTx("test2", big.NewInt(20), uint64(29))
	require.Nil(t, ret)
	_, exist := pi.pendingInboundsBnB.Load("test2")
	require.True(t, exist)

	// now we process the tx with two top-up
	coin = sdk.Coin{
		Denom:  InBoundDenomTest2,
		Amount: sdk.NewIntFromUint64(32),
	}

	feeCoin = sdk.Coin{
		Denom:  config.InBoundDenomFee,
		Amount: sdk.NewIntFromUint64(0),
	}
	btx = InboundTx{
		"testTxID",
		accs[1].oppyAddr,
		uint64(10),
		coin,
		feeCoin,
	}
	pi.pendingInbounds.Store("test2", &btx)

	ret = pi.updateInboundTx("test2", big.NewInt(1), uint64(32))
	require.Nil(t, ret)
	//
	// if we do not have enough fee paid
	ret = pi.updateInboundTx("test2", big.NewInt(8), uint64(33))
	require.Nil(t, ret)

	ret = pi.updateInboundTx("test2", big.NewInt(1), uint64(34))
	require.Equal(t, ret.Address.String(), accs[1].oppyAddr.String())
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

var (
	testKey, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAddr    = crypto.PubkeyToAddress(testKey.PublicKey)
	testBalance = big.NewInt(200e15)
)

var genesis = &core.Genesis{
	Config:    params.AllEthashProtocolChanges,
	Alloc:     core.GenesisAlloc{testAddr: {Balance: testBalance}},
	ExtraData: []byte("test genesis"),
	Timestamp: 9000,
	BaseFee:   big.NewInt(1),
}

func generateTestChain(testTx []*ethTypes.Transaction) []*ethTypes.Block {
	db := rawdb.NewMemoryDatabase()
	generate := func(i int, g *core.BlockGen) {
		g.OffsetTime(5)
		g.SetExtra([]byte("test"))
		if i == 1 {
			for _, el := range testTx {
				g.AddTx(el)
			}
		}
	}
	gblock := genesis.ToBlock(db)
	engine := ethash.NewFaker()
	blocks, _ := core.GenerateChain(genesis.Config, gblock, engine, db, len(testTx), generate)
	blocks = append([]*ethTypes.Block{gblock}, blocks...)
	return blocks
}

func newTestBackend(t *testing.T, txs []*ethTypes.Transaction) (*node.Node, []*ethTypes.Block) {
	// Generate test chain.

	blocks := generateTestChain(txs)

	// Create node
	n, err := node.New(&node.Config{})
	if err != nil {
		t.Fatalf("can't create new node: %v", err)
	}
	// Create Ethereum Service
	config := &ethconfig.Config{Genesis: genesis}
	config.Ethash.PowMode = ethash.ModeFake
	ethservice, err := eth.New(n, config)
	if err != nil {
		t.Fatalf("can't create new ethereum service: %v", err)
	}
	// Import the test chain.
	if err := n.Start(); err != nil {
		t.Fatalf("can't start test node: %v", err)
	}
	if _, err := ethservice.BlockChain().InsertChain(blocks[1:]); err != nil {
		t.Fatalf("can't import test blocks: %v", err)
	}
	return n, blocks
}

func TestProcessEachBlock(t *testing.T) {
	InBoundDenomTest := "JUSD"
	tl, err := createMockTokenlist("testJUSDAddr", "JUSD")
	assert.Nil(t, err)
	misc.SetupBech32Prefix()
	accs, err := generateRandomPrivKey(2)
	assert.Nil(t, err)
	emptyEip2718Tx := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		ChainID:  big.NewInt(1337),
		Nonce:    0,
		To:       &accs[0].commAddr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("wrongTx"),
	})

	emptyEip2718TxNotToBridge := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		ChainID:  big.NewInt(1337),
		Nonce:    1,
		To:       &accs[1].commAddr,
		Value:    big.NewInt(10),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("testerTx"),
	})

	// fee, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	emptyEip2718TxGoodTopUpFee := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		ChainID:  big.NewInt(1337),
		Nonce:    2,
		To:       &accs[0].commAddr,
		Value:    big.NewInt(8),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("test1"),
	})

	// fee, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	emptyEip2718TxGoodTopUpEmptyData := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		ChainID:  big.NewInt(1337),
		Nonce:    3,
		To:       &accs[0].commAddr,
		Value:    big.NewInt(8),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     nil,
	})

	// fee, err := sdk.NewDecFromStr(config.InBoundFeeMin)
	emptyEip2718TxGoodTopUpFeeBeforeERC20 := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		ChainID:  big.NewInt(1337),
		Nonce:    4,
		To:       &accs[0].commAddr,
		Value:    big.NewInt(8),
		Gas:      25000,
		GasPrice: big.NewInt(1),
		Data:     []byte("ERC20NOTREADY"),
	})

	tBlock := ethTypes.Block{}

	tAbi, err := abi.JSON(strings.NewReader(generated.TokenMetaData.ABI))
	assert.Nil(t, err)

	allTxs := []*ethTypes.Transaction{emptyEip2718Tx, emptyEip2718TxNotToBridge, emptyEip2718TxGoodTopUpFee, emptyEip2718TxGoodTopUpEmptyData, emptyEip2718TxGoodTopUpFeeBeforeERC20}
	backend, _ := newTestBackend(t, allTxs)
	client, _ := backend.Attach()
	defer backend.Close()
	defer client.Close()
	c := ethclient.NewClient(client)
	pi := Instance{
		EthClient:          c,
		ethClientLocker:    &sync.RWMutex{},
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		tokenAbi:           &tAbi,
		RetryInboundReq:    &sync.Map{},
		InboundReqChan:     make(chan *common2.InBoundReq, 1),
		TokenList:          tl,
	}

	coin := sdk.Coin{
		Denom:  InBoundDenomTest,
		Amount: sdk.NewIntFromUint64(32),
	}

	feeCoin := sdk.Coin{
		Denom:  config.InBoundDenomFee,
		Amount: sdk.NewIntFromUint64(1),
	}

	btx := InboundTx{
		"testTxID",
		accs[0].oppyAddr,
		uint64(10),
		coin,
		feeCoin,
	}
	pi.pendingInbounds.Store(hex.EncodeToString([]byte("test1")), &btx)

	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[0].pk,
			PoolAddr:   accs[0].oppyAddr,
		},
	}

	err = pi.UpdatePool(&poolInfo)
	require.Nil(t, err)
	pi.processEachBlock(&tBlock, 10)
	ret, exist := pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	// indicate nothing happens
	require.True(t, exist)
	storedInbound := ret.(*InboundTx)
	require.Equal(t, storedInbound.Address.String(), accs[0].oppyAddr.String())

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
	storedInbound = ret.(*InboundTx)
	require.Equal(t, storedInbound.Address.String(), accs[0].oppyAddr.String())

	// check not to bridge
	tBlock2 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxNotToBridge}, nil, nil, newHasher())
	pi.processEachBlock(tBlock2, 10)
	ret, exist = pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	require.True(t, exist)
	storedInbound = ret.(*InboundTx)
	require.Equal(t, storedInbound.Fee.Amount.String(), feeCoin.Amount.String())

	//
	// now we top up the fee
	tBlock3 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpFee}, nil, nil, newHasher())
	pi.processEachBlock(tBlock3, 10)

	ret, exist = pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	require.True(t, exist)
	storedInbound = ret.(*InboundTx)
	topupFee := sdk.NewIntFromBigInt(emptyEip2718TxGoodTopUpFee.Value())
	require.True(t, storedInbound.Fee.Amount.Equal(topupFee.Add(feeCoin.Amount)))

	tBlock3 = ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpEmptyData}, nil, nil, newHasher())
	pi.processEachBlock(tBlock3, 10)

	ret, exist = pi.pendingInbounds.Load(hex.EncodeToString([]byte("test1")))
	require.True(t, exist)
	storedInbound = ret.(*InboundTx)
	require.True(t, storedInbound.Fee.Amount.Equal(topupFee.Add(feeCoin.Amount)))
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
	data := ret.(*InboundTxBnb)
	assert.Equal(t, data.Fee.Amount.String(), emptyEip2718TxGoodTopUpFeeBeforeERC20.Value().String())
}

func TestProcessEachBlockErc20(t *testing.T) {
	accs, err := generateRandomPrivKey(3)
	assert.Nil(t, err)
	tAbi, err := abi.JSON(strings.NewReader(generated.TokenMetaData.ABI))
	assert.Nil(t, err)
	tl, err := createMockTokenlist(accs[1].commAddr.String(), "test")
	assert.Nil(t, err)
	pi := Instance{
		ethClientLocker:    &sync.RWMutex{},
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		tokenAbi:           &tAbi,
		InboundReqChan:     make(chan *common2.InBoundReq, 1),
		TokenList:          tl,
	}

	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: accs[0].pk,
			PoolAddr:   accs[0].oppyAddr,
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
	// this address should be the same as the pool address as ERC20 contract definition
	dataRaw, err := method.Inputs.Pack(accs[1].commAddr, big.NewInt(10))
	assert.Nil(t, err)
	// we need to put 4 leading 0 to match the format in test
	data := append([]byte("0000"), dataRaw...)

	// sk, err := crypto.ToECDSA(accs[0].sk.Bytes())
	// assert.Nil(t, err)
	// data = nil
	Eip2718Tx := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.LegacyTx{
		Nonce:    0,
		To:       &accs[2].commAddr,
		Value:    big.NewInt(10),
		Gas:      params.CallNewAccountGas,
		GasPrice: big.NewInt(1),
		Data:     data,
	})

	// though to the token addr but not to the pool, so we ignore this tx
	Eip2718TxNotPool := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		Nonce:    1,
		To:       &accs[1].commAddr,
		Value:    big.NewInt(10),
		Gas:      params.CallNewAccountGas,
		GasPrice: big.NewInt(1),
		Data:     data,
	})

	dataToSign := []byte("hello")
	hash := crypto.Keccak256Hash(dataToSign)
	signature, err := crypto.Sign(hash.Bytes(), testKey)
	assert.Nil(t, err)
	r := signature[:32]
	s := signature[32:64]
	v := signature[64:65]

	// though to the token addr but not to the pool, so we ignore this tx
	Eip2718TxGoodPass := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		Nonce:    2,
		To:       &accs[1].commAddr,
		Value:    big.NewInt(10),
		Gas:      params.CallNewAccountGas,
		GasPrice: big.NewInt(1),
		Data:     data,
		R:        new(big.Int).SetBytes(r),
		S:        new(big.Int).SetBytes(s),
		V:        new(big.Int).SetBytes(v),
	})

	mySk := hex.EncodeToString(accs[0].sk.Bytes())
	testKey, err = crypto.HexToECDSA(mySk)
	testAddr = crypto.PubkeyToAddress(testKey.PublicKey)
	fmt.Printf(">>>%v\n", testAddr.String())
	assert.NoError(t, err)
	backend, _ := newTestBackend(t, []*ethTypes.Transaction{Eip2718Tx, Eip2718TxNotPool, Eip2718TxGoodPass})
	defer backend.Close()
	client, err := backend.Attach()
	assert.NoError(t, err)
	defer client.Close()
	c := ethclient.NewClient(client)
	pi.EthClient = c

	// since token addr is not set, so the system should not put this tx in top-up queue
	tBlock := ethTypes.NewBlock(header, []*ethTypes.Transaction{Eip2718Tx}, nil, nil, newHasher())
	pi.processEachBlock(tBlock, 10)
	counter := 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter++
		return true
	})
	assert.Equal(t, counter, 0)

	tBlock = ethTypes.NewBlock(header, []*ethTypes.Transaction{Eip2718TxNotPool}, nil, nil, newHasher())
	pi.processEachBlock(tBlock, 10)
	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter++
		return true
	})
	assert.Equal(t, counter, 0)

	// dataRaw, err = method.Inputs.Pack(accs[0].commAddr, big.NewInt(10))
	// assert.Nil(t, err)

	// we need to put 4 leading 0 to match the format in test
	// data = append([]byte("0000"), dataRaw...)

	tBlock = ethTypes.NewBlock(header, []*ethTypes.Transaction{Eip2718TxGoodPass}, nil, nil, newHasher())

	// we test that the tx is not to the pool
	pi.processEachBlock(tBlock, 10)

	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter++
		return true
	})
	assert.Equal(t, counter, 0)

	mockPoolInfo := vaulttypes.PoolInfo{BlockHeight: "10", CreatePool: &vaulttypes.PoolProposal{PoolPubKey: accs[1].pk, PoolAddr: accs[1].oppyAddr}}
	err = pi.UpdatePool(&mockPoolInfo)
	assert.Nil(t, err)
	err = pi.UpdatePool(&mockPoolInfo)
	assert.Nil(t, err)
	pi.processEachBlock(tBlock, 10)
	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter++
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
	btx1 := InboundTx{
		"testTxID1",
		acc[0].oppyAddr,
		firstBlock,
		coin,
		coin,
	}

	btx2 := InboundTx{
		"testTxID2",
		acc[0].oppyAddr,
		secondBlock,
		coin,
		coin,
	}

	btx3 := InboundTx{
		"testTxID3",
		acc[0].oppyAddr,
		thirdBlock,
		coin,
		coin,
	}

	pi := Instance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		InboundReqChan:     make(chan *common2.InBoundReq, 1),
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
	btx1 := InboundTxBnb{
		firstBlock,
		"bnb1",
		coin,
	}

	btx2 := InboundTxBnb{
		secondBlock,
		"bnb1",
		coin,
	}

	btx3 := InboundTxBnb{
		thirdBlock,
		"bnb1",
		coin,
	}

	pi := Instance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		InboundReqChan:     make(chan *common2.InBoundReq, 1),
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
	tl, err := createMockTokenlist(accs[0].commAddr.String(), "test")
	assert.Nil(t, err)
	pi := Instance{
		lastTwoPools:       make([]*common2.PoolInfo, 2),
		poolLocker:         &sync.RWMutex{},
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
		InboundReqChan:     make(chan *common2.InBoundReq, 1),
		TokenList:          tl,
	}

	bnbTx := InboundTxBnb{
		uint64(11),
		hex.EncodeToString([]byte("test1")),
		sdk.NewCoin(config.InBoundDenomFee, sdk.NewIntFromUint64(8)),
	}

	pi.pendingInboundsBnB.Store(hex.EncodeToString([]byte("test1")), &bnbTx)
	err = pi.processInboundTx(hex.EncodeToString([]byte("test1")), uint64(10), accs[1].oppyAddr, accs[2].commAddr, big.NewInt(11), accs[0].commAddr)
	assert.NoError(t, err)

	counter := 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter++
		return true
	})

	counter2 := 0
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		counter2++
		return true
	})

	assert.Equal(t, 0, counter2)

	pi.pendingInbounds = &sync.Map{}
	pi.pendingInboundsBnB = &sync.Map{}
	bnbTx2 := InboundTxBnb{
		uint64(11),
		hex.EncodeToString([]byte("test2")),
		sdk.NewCoin(config.InBoundDenomFee, sdk.NewIntFromUint64(18)),
	}

	pi.pendingInboundsBnB.Store(hex.EncodeToString([]byte("test2")), &bnbTx2)
	err = pi.processInboundTx(hex.EncodeToString([]byte("test2")), uint64(10), accs[1].oppyAddr, accs[2].commAddr, big.NewInt(11), accs[0].commAddr)
	assert.NoError(t, err)

	counter = 0
	pi.pendingInbounds.Range(func(key, value interface{}) bool {
		counter++
		return true
	})
	assert.Equal(t, 0, counter)

	counter2 = 0
	pi.pendingInboundsBnB.Range(func(key, value interface{}) bool {
		counter2++
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

	addr, err := sdk.AccAddressFromBech32("oppy1rfmwldwrm3652shx3a7say0v4vvtglasncg0uu")
	assert.NoError(t, err)
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
			a := &InboundTx{
				Address: tt.fields.address,
				Token:   tt.fields.token,
				Fee:     tt.fields.fee,
			}
			if err := a.Verify(); (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

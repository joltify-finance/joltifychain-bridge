package pubchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"hash"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/core/rawdb"

	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
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
	acc, err := generateRandomPrivKey(3)
	assert.Nil(t, err)
	tssServer := TssMock{acc[0].sk}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testDenom"}, []string{"testAddr"})
	assert.Nil(t, err)
	wg := sync.WaitGroup{}
	pi, err := NewChainInstance(misc.WebsocketTest, &tssServer, tl, &wg)
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

	erc20Tx := Erc20TxInfo{
		fromAddr:     sdk.AccAddress{},
		tokenAddress: nonExistedTokenAddr,
		toAddr:       transferTo,
		Amount:       testAmount,
	}

	err = pi.ProcessInBoundERC20(tx, &erc20Tx, uint64(10))
	require.EqualError(t, err, "token is not on our token list")

	ExistedTokenAddrStr := "0x15fb343d82cD1C22542261dF408dA8396A829F6B"
	ExistedTokenAddr := common.HexToAddress(ExistedTokenAddrStr)
	pi.TokenList, err = tokenlist.CreateMockTokenlist([]string{ExistedTokenAddrStr}, []string{"testDenom"})
	assert.Nil(t, err)

	erc20Tx.tokenAddress = ExistedTokenAddr

	err = pi.ProcessInBoundERC20(tx, &erc20Tx, uint64(10))
	require.Nil(t, err)

	pi.RetryInboundReq.Range(func(key, value any) bool {
		data := value.(*common2.InBoundReq)
		expected := sdk.Coin{Denom: "testdenom", Amount: sdk.NewInt(100)}
		assert.True(t, data.Coin.Equal(expected))
		return true
	})
	wg.Wait()
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

//
//func generateTestChain(testTx []*ethTypes.Transaction) []*ethTypes.Block {
//	db := rawdb.NewMemoryDatabase()
//	generate := func(i int, g *core.BlockGen) {
//		g.OffsetTime(5)
//		g.SetExtra([]byte("test"))
//		if i == 1 {
//			for _, el := range testTx {
//				g.AddTx(el)
//			}
//		}
//	}
//	gblock := genesis.ToBlock(db)
//	engine := ethash.NewFaker()
//	blocks, _ := core.GenerateChain(genesis.Config, gblock, engine, db, len(testTx), generate)
//	blocks = append([]*ethTypes.Block{gblock}, blocks...)
//	return blocks
//}

func generateTestChain(txs []*ethtypes.Transaction) []*ethtypes.Block {
	db := rawdb.NewMemoryDatabase()
	generate := func(i int, g *core.BlockGen) {
		g.OffsetTime(5)
		g.SetExtra([]byte("test"))
		if i == 1 {
			// Test transactions are included in block #2.
			for _, el := range txs {
				g.AddTx(el)
			}
		}
	}

	gblock := genesis.MustCommit(db)
	engine := ethash.NewFaker()
	blocks, _ := core.GenerateChain(genesis.Config, gblock, engine, db, 2, generate)
	blocks = append([]*ethtypes.Block{gblock}, blocks...)
	return blocks
}

func newTestBackend(t *testing.T, txs []*ethTypes.Transaction) (*node.Node, []*ethtypes.Block) {
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
	tl, err := tokenlist.CreateMockTokenlist([]string{"testJUSDAddr"}, []string{"JUSD"})
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

	tAbi, err := abi.JSON(strings.NewReader(generated.GeneratedMetaData.ABI))
	assert.Nil(t, err)

	allTxs := []*ethTypes.Transaction{emptyEip2718Tx, emptyEip2718TxNotToBridge, emptyEip2718TxGoodTopUpFee, emptyEip2718TxGoodTopUpEmptyData, emptyEip2718TxGoodTopUpFeeBeforeERC20}
	backend, _ := newTestBackend(t, allTxs)
	client, _ := backend.Attach()
	defer backend.Close()
	defer client.Close()
	c := ethclient.NewClient(client)
	pi := Instance{
		EthClient:       c,
		ethClientLocker: &sync.RWMutex{},
		lastTwoPools:    make([]*common2.PoolInfo, 2),
		poolLocker:      &sync.RWMutex{},
		tokenAbi:        &tAbi,
		RetryInboundReq: &sync.Map{},
		InboundReqChan:  make(chan []*common2.InBoundReq, 1),
		TokenList:       tl,
	}

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
	// indicate nothing happens

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

	// check not to bridge
	tBlock2 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxNotToBridge}, nil, nil, newHasher())
	pi.processEachBlock(tBlock2, 10)

	//
	// now we top up the fee
	tBlock3 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpFee}, nil, nil, newHasher())
	pi.processEachBlock(tBlock3, 10)

	tBlock3 = ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpEmptyData}, nil, nil, newHasher())
	pi.processEachBlock(tBlock3, 10)

	//
	tBlock3 = ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpFee}, nil, nil, newHasher())
	pi.processEachBlock(tBlock3, 10)

	//
	//// now we top up the fee before ERC20 tx arrive
	tBlock4 := ethTypes.NewBlock(header, []*ethTypes.Transaction{emptyEip2718TxGoodTopUpFeeBeforeERC20}, nil, nil, newHasher())
	pi.processEachBlock(tBlock4, 10)
}

func TestProcessEachBlockErc20(t *testing.T) {
	accs, err := generateRandomPrivKey(5)
	assert.Nil(t, err)
	tAbi, err := abi.JSON(strings.NewReader(generated.GeneratedMetaData.ABI))
	assert.Nil(t, err)
	tl, err := tokenlist.CreateMockTokenlist([]string{accs[0].commAddr.String()}, []string{"abnb"})
	assert.Nil(t, err)

	tl2, err := tokenlist.CreateMockTokenlist([]string{accs[4].commAddr.String()}, []string{"testtoken"})
	assert.Nil(t, err)

	tl3, err := tokenlist.CreateMockTokenlist([]string{"native"}, []string{"abnb"})
	assert.Nil(t, err)
	pi := Instance{
		ethClientLocker: &sync.RWMutex{},
		lastTwoPools:    make([]*common2.PoolInfo, 2),
		poolLocker:      &sync.RWMutex{},
		tokenAbi:        &tAbi,
		InboundReqChan:  make(chan []*common2.InBoundReq, 1),
		TokenList:       tl,
		RetryInboundReq: &sync.Map{},
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

	method, ok := tAbi.Methods["oppyTransfer"]
	assert.True(t, ok)

	// address toAddress, uint256 amount, address contractAddress, bytes memo
	// this address should be the same as the pool address as ERC20 contract definition

	memo := common2.BridgeMemo{
		Dest: accs[3].oppyAddr.String(),
	}

	memoByte, err := json.Marshal(memo)
	assert.Nil(t, err)

	dataRaw, err := method.Inputs.Pack(accs[1].commAddr, big.NewInt(10), accs[4].commAddr, memoByte)
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
	hashData := crypto.Keccak256Hash(dataToSign)
	signature, err := crypto.Sign(hashData.Bytes(), testKey)
	assert.Nil(t, err)
	r := signature[:32]
	s := signature[32:64]
	v := signature[64:65]

	contractAddressEth := common.HexToAddress(OppyContractAddress)
	// though to the token addr but not to the pool, so we ignore this tx
	Eip2718TxGoodPass := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		Nonce:    2,
		To:       &contractAddressEth,
		Value:    big.NewInt(10),
		Gas:      params.CallNewAccountGas,
		GasPrice: big.NewInt(1),
		Data:     data,
		R:        new(big.Int).SetBytes(r),
		S:        new(big.Int).SetBytes(s),
		V:        new(big.Int).SetBytes(v),
	})

	// though to the token addr but not to the pool, so we ignore this tx
	txNative := ethTypes.MustSignNewTx(testKey, ethTypes.LatestSigner(genesis.Config), &ethTypes.AccessListTx{
		Nonce:    3,
		To:       &accs[1].commAddr,
		Value:    big.NewInt(101),
		Gas:      params.CallNewAccountGas,
		GasPrice: big.NewInt(1),
		Data:     memoByte,
		R:        new(big.Int).SetBytes(r),
		S:        new(big.Int).SetBytes(s),
		V:        new(big.Int).SetBytes(v),
	})

	mySk := hex.EncodeToString(accs[0].sk.Bytes())
	testKey, err = crypto.HexToECDSA(mySk)
	testAddr = crypto.PubkeyToAddress(testKey.PublicKey)
	assert.NoError(t, err)
	backend, _ := newTestBackend(t, []*ethTypes.Transaction{Eip2718Tx, Eip2718TxNotPool, Eip2718TxGoodPass, txNative})
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
	pi.RetryInboundReq.Range(func(key, value any) bool {
		counter++
		return true
	})
	assert.Equal(t, counter, 0)
	tBlock = ethTypes.NewBlock(header, []*ethTypes.Transaction{Eip2718TxNotPool}, nil, nil, newHasher())
	pi.processEachBlock(tBlock, 10)
	pi.RetryInboundReq.Range(func(key, value any) bool {
		counter++
		return true
	})
	assert.Equal(t, counter, 0)
	pi.TokenList = tl2

	tBlock = ethTypes.NewBlock(header, []*ethTypes.Transaction{Eip2718TxGoodPass}, nil, nil, newHasher())
	// we test that the tx is not to the pool
	pi.processEachBlock(tBlock, 10)
	pi.RetryInboundReq.Range(func(key, value any) bool {
		panic("should not have items")
	})

	mockPoolInfo := vaulttypes.PoolInfo{BlockHeight: "10", CreatePool: &vaulttypes.PoolProposal{PoolPubKey: accs[1].pk, PoolAddr: accs[1].oppyAddr}}
	err = pi.UpdatePool(&mockPoolInfo)
	assert.Nil(t, err)
	err = pi.UpdatePool(&mockPoolInfo)
	assert.Nil(t, err)
	pi.processEachBlock(tBlock, 10)
	pi.RetryInboundReq.Range(func(key, value any) bool {
		counter++
		return true
	})
	assert.Equal(t, counter, 1)

	// now we process the native token
	pi.TokenList = tl3
	tBlock = ethTypes.NewBlock(header, []*ethTypes.Transaction{txNative}, nil, nil, newHasher())
	// we test that the tx is not to the pool
	pi.processEachBlock(tBlock, 10)
	var storedToken sdk.Coins
	pi.RetryInboundReq.Range(func(key, value any) bool {
		data := value.(*common2.InBoundReq)
		storedToken = append(storedToken, data.Coin)
		return true
	})

	// we should have native and erc20 token request in retry request
	expected := []sdk.Coin{{Denom: "testtoken", Amount: sdk.NewInt(10)}, {Denom: "abnb", Amount: sdk.NewInt(101)}}
	assert.True(t, storedToken.IsEqual(expected))
}

func TestProcessERC20InBoundOnBSC(t *testing.T) {
	misc.SetupBech32Prefix()
	OppyContractAddress = "0x94277968dff216265313657425d9d7577ad32dd1"
	acc, err := generateRandomPrivKey(3)
	assert.Nil(t, err)
	tssServer := TssMock{acc[0].sk}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testDenom"}, []string{"testAddr"})
	assert.Nil(t, err)
	wg := sync.WaitGroup{}
	pi, err := NewChainInstance(misc.WebsocketTest, &tssServer, tl, &wg)
	assert.Nil(t, err)

	// TODO since our bsc testnet is the pruned node, it can only store the latest tx
	txid := common.HexToHash("0x6a8effcfd4a01e3e6f09e1a026da28914d49d4adc20a84ad331f70fd4302e4b1")
	tx, isPending, err := pi.EthClient.TransactionByHash(context.Background(), txid)
	assert.Nil(t, err)
	assert.False(t, isPending)

	tAbi, err := abi.JSON(strings.NewReader(generated.GeneratedMetaData.ABI))
	assert.Nil(t, err)
	pi.tokenAbi = &tAbi
	txInfo, err := pi.checkErc20(tx.Data(), tx.To().Hex())
	assert.Nil(t, err)
	assert.Equal(t, txInfo.fromAddr.String(), "oppy1txtsnx4gr4effr8542778fsxc20j5vzq7wu7r7")
	wg.Wait()
}

func TestProcessNativeInBoundOnBSC(t *testing.T) {
	misc.SetupBech32Prefix()
	acc, err := generateRandomPrivKey(3)
	assert.Nil(t, err)
	tssServer := TssMock{acc[0].sk}
	tl, err := tokenlist.CreateMockTokenlist([]string{"native"}, []string{"abnb"})
	assert.Nil(t, err)
	wg := sync.WaitGroup{}
	pi, err := NewChainInstance(misc.WebsocketTest, &tssServer, tl, &wg)

	poolInfo := common2.PoolInfo{
		EthAddress: common.HexToAddress("0x1f65cc33558b6825db119e9fe4c73b436211667e"),
	}

	pi.lastTwoPools = []*common2.PoolInfo{&poolInfo, &poolInfo}
	assert.Nil(t, err)

	// TODO need to update the hight as our bsc testnode is the pruned one
	b, err := pi.EthClient.BlockByNumber(context.Background(), big.NewInt(22997528))
	assert.Nil(t, err)

	tAbi, err := abi.JSON(strings.NewReader(generated.GeneratedMetaData.ABI))
	assert.Nil(t, err)
	pi.tokenAbi = &tAbi
	pi.processEachBlock(b, 101)

	pi.RetryInboundReq.Range(func(key, value any) bool {
		item := value.(*common2.InBoundReq)
		assert.Equal(t, item.Coin.Denom, "abnb")
		assert.Equal(t, item.Coin.Amount.String(), "1234567890000000")
		assert.Equal(t, common.BytesToHash(item.TxID).String(), "0xfe172d0a5c4344a31cab330163861c8fbd257fdb5ff31759abb52e63043ef396")
		assert.Equal(t, item.Address.String(), "oppy12gflne2dadajtzn5h7x8z4udfpd7f22dvly0zg")
		return true
	})
	wg.Wait()
}

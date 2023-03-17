package cosbridge

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/ethereum/go-ethereum/common"
	grpc1 "github.com/gogo/protobuf/grpc"
	common2 "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/tokenlist"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/joltify-finance/joltify_lending/testutil/network"
	pricefeedtypes "github.com/joltify-finance/joltify_lending/x/third_party/pricefeed/types"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

type OutBoundTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
	grpc        grpc1.ClientConn
}

const (
	AddrJUSD = "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"
)

func (o *OutBoundTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.BondDenom = "stake"
	cfg.MinGasPrices = "0stake"
	cfg.BondedTokens = sdk.NewInt(10000000000000000)
	cfg.StakingTokens = sdk.NewInt(100000000000000000)
	cfg.ChainID = config.ChainID
	o.cfg = cfg
	o.validatorky = keyring.NewInMemory()
	// now we put the mock pool list in the test
	state := vaulttypes.GenesisState{}
	stateStaking := stakingtypes.GenesisState{}

	// we add the price for the tokens
	priceFeed := pricefeedtypes.GenesisState{}

	bnbPrice := pricefeedtypes.PostedPrice{
		MarketID:      "bnb:usd",
		OracleAddress: sdk.AccAddress("mock"),
		Price:         sdk.NewDecWithPrec(2571, 1),
		Expiry:        time.Now().Add(time.Hour),
	}

	joltPrice := pricefeedtypes.PostedPrice{
		MarketID:      "jolt:usd",
		OracleAddress: sdk.AccAddress("mock"),
		Price:         sdk.NewDecWithPrec(12, 1),
		Expiry:        time.Now().Add(time.Hour),
	}

	priceFeed.PostedPrices = pricefeedtypes.PostedPrices{bnbPrice, joltPrice}
	priceFeed.Params = pricefeedtypes.Params{Markets: pricefeedtypes.GenDefaultMarket()}

	bufPriceFeed, err := cfg.Codec.MarshalJSON(&priceFeed)
	o.Require().NoError(err)
	cfg.GenesisState[pricefeedtypes.ModuleName] = bufPriceFeed

	o.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[vaulttypes.ModuleName], &state))
	o.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateStaking))

	validators, err := genNValidator(3, o.validatorky)
	o.Require().NoError(err)
	for i := 1; i < 5; i++ {
		randPoolSk := ed25519.GenPrivKey()
		poolPubKey, err := legacybech32.MarshalPubKey(legacybech32.AccPK, randPoolSk.PubKey()) // nolint
		o.Require().NoError(err)

		var nodes []sdk.AccAddress
		for _, el := range validators {
			operator, err := sdk.ValAddressFromBech32(el.OperatorAddress)
			if err != nil {
				panic(err)
			}
			nodes = append(nodes, operator.Bytes())
		}
		pro := vaulttypes.PoolProposal{
			PoolPubKey: poolPubKey,
			PoolAddr:   randPoolSk.PubKey().Address().Bytes(),
			Nodes:      nodes,
		}
		state.CreatePoolList = append(state.CreatePoolList, &vaulttypes.CreatePool{BlockHeight: strconv.Itoa(i), Validators: validators, Proposal: []*vaulttypes.PoolProposal{&pro}})
	}
	state.LatestTwoPool = state.CreatePoolList[:2]
	testToken := vaulttypes.IssueToken{
		Index: "testindex",
	}
	state.IssueTokenList = append(state.IssueTokenList, &testToken)

	buf, err := cfg.Codec.MarshalJSON(&state)
	o.Require().NoError(err)
	cfg.GenesisState[vaulttypes.ModuleName] = buf

	var stateVault stakingtypes.GenesisState
	o.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateVault))
	stateVault.Params.MaxValidators = 3
	state.Params.BlockChurnInterval = 1
	buf, err = cfg.Codec.MarshalJSON(&stateVault)
	o.Require().NoError(err)
	cfg.GenesisState[stakingtypes.ModuleName] = buf

	o.network = network.New(o.T(), cfg)

	o.Require().NotNil(o.network)

	_, err = o.network.WaitForHeight(5)
	o.Require().Nil(err)
	o.grpc = o.network.Validators[0].ClientCtx
	o.queryClient = tmservice.NewServiceClient(o.network.Validators[0].ClientCtx)
}

type Account struct {
	sk       *secp256k1.PrivKey
	pk       string
	joltAddr sdk.AccAddress
	commAddr common.Address
}

func generateRandomPrivKey(n int) ([]Account, error) {
	randomAccounts := make([]Account, n)
	for i := 0; i < n; i++ {
		sk := secp256k1.GenPrivKey()
		pk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, sk.PubKey()) // nolint

		ethAddr, err := misc.PoolPubKeyToEthAddress(pk)
		if err != nil {
			return nil, err
		}
		addrOppy, err := sdk.AccAddressFromHex(sk.PubKey().Address().String())
		if err != nil {
			return nil, err
		}
		tAccount := Account{
			sk,
			pk,
			addrOppy,
			ethAddr,
		}
		randomAccounts[i] = tAccount
	}
	return randomAccounts, nil
}

func (o OutBoundTestSuite) TestUpdatePool() {
	var err error
	accs, err := generateRandomPrivKey(2)
	o.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}
	//
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"}, []string{"BSC"})
	o.Require().NoError(err)
	rp := common2.NewRetryPools()
	oc, err := NewJoltifyBridge(o.network.Validators[0].APIAddress, o.network.Validators[0].RPCAddress, o.network.Validators[0].ClientCtx, &tss, tl, rp)
	o.Require().NoError(err)
	defer func() {
		err := oc.TerminateBridge()
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()
	//
	key, err := oc.CosHandler.NewMnemonic("pooltester1")
	o.Require().NoError(err)

	key2, err := oc.CosHandler.NewMnemonic("pooltester2")
	o.Require().NoError(err)

	key3, err := oc.CosHandler.NewMnemonic("pooltester3")
	o.Require().NoError(err)

	cospk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, key.GetPubKey()) // nolint

	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: cospk,
			PoolAddr:   accs[0].joltAddr,
		},
	}

	oc.UpdatePool(&poolInfo)
	pubkeyStr := key.GetPubKey().Address().String()
	pk, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	pools := oc.GetPool()
	o.Require().Nil(pools[0])
	addr2 := pools[1].CosAddress
	o.Require().True(pk.Equals(addr2))
	// now we add another pool
	cospk = legacybech32.MustMarshalPubKey(legacybech32.AccPK, key2.GetPubKey()) // nolint

	poolInfo = vaulttypes.PoolInfo{
		BlockHeight: "101",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: cospk,
			PoolAddr:   accs[0].joltAddr,
		},
	}
	oc.UpdatePool(&poolInfo)
	pools = oc.GetPool()
	pubkeyStr = key2.GetPubKey().Address().String()
	pk2, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	o.Require().True(pk.Equals(pools[0].CosAddress))

	pool1 := oc.lastTwoPools[1].CosAddress
	o.Require().True(pk2.Equals(pool1))

	// now we add another pool and pop out the firt one

	cospk = legacybech32.MustMarshalPubKey(legacybech32.AccPK, key3.GetPubKey()) // nolint

	poolInfo = vaulttypes.PoolInfo{
		BlockHeight: "102",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: cospk,
			PoolAddr:   accs[0].joltAddr,
		},
	}
	oc.UpdatePool(&poolInfo)
	pools = oc.GetPool()

	pubkeyStr = key3.GetPubKey().Address().String()
	pk3, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	o.Require().True(pk3.Equals(pools[1].CosAddress))

	time.Sleep(time.Second)
}

func (o OutBoundTestSuite) TestOutBoundReq() {
	accs, err := generateRandomPrivKey(2)

	o.Require().NoError(err)
	boundReq := common2.NewOutboundReq("testID", accs[0].commAddr.Bytes(), accs[1].commAddr.Bytes(), sdk.NewCoin("JUSD", sdk.NewInt(1)), AddrJUSD, 101, nil, "BSC", true)
	boundReq.SetItemNonce(accs[1].commAddr.Bytes(), 100, "", 0)
	a, b, _, amount, h := boundReq.GetOutBoundInfo()
	o.Require().True(bytes.Equal(a, accs[0].commAddr.Bytes()))
	o.Require().True(bytes.Equal(b, accs[1].commAddr.Bytes()))
	o.Require().Equal(amount.String(), "1")
	o.Require().Equal(h, uint64(100))
}

func (o OutBoundTestSuite) TestProcessMsg() {
	accs, err := generateRandomPrivKey(4)
	o.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr", "native"}, []string{"abnb", "ujolt"}, []string{"BSC", "BSC"})
	o.Require().NoError(err)
	rp := common2.NewRetryPools()
	oc, err := NewJoltifyBridge(o.network.Validators[0].RPCAddress, o.network.Validators[0].RPCAddress, o.network.Validators[0].ClientCtx, &tss, tl, rp)
	o.Require().NoError(err)
	defer func() {
		err2 := oc.TerminateBridge()
		if err2 != nil {
			oc.logger.Error().Err(err2).Msgf("fail to terminate the bridge")
		}
	}()

	baseBlockHeight := int64(100)
	msg := banktypes.MsgSend{}
	memo := common2.BridgeMemo{
		Dest:      accs[0].commAddr.String(),
		ChainType: "BSC",
	}

	poolInfo := &common2.PoolInfo{
		Pk:         accs[3].pk,
		CosAddress: accs[3].joltAddr,
		EthAddress: accs[3].commAddr,
	}

	_, err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, poolInfo, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "zero amount")

	msg.FromAddress = o.network.Validators[0].Address.String()
	_, err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, poolInfo, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "zero amount")

	ret := oc.CosHandler.CheckWhetherIssueTokenAlreadyExist(o.grpc, "testindex")
	o.Require().True(ret)

	msg.ToAddress = accs[3].joltAddr.String()
	_, err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, poolInfo, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "zero amount")

	msg.ToAddress = accs[1].joltAddr.String()
	_, err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, poolInfo, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "zero amount")

	coin2 := sdk.NewCoin("invalidToken", sdk.NewInt(1))
	coin3 := sdk.NewCoin("invalidToken2", sdk.NewInt(100))

	msg.Amount = sdk.Coins{coin2, coin3}
	_, err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, poolInfo, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "token is not on our token list")

	// test ERC20 token
	txID := "5dd520d7ebcd1fc1c070d0c595839991c544cc45dcdbfa43aa86370daa258676"
	txIDByte, err := hex.DecodeString(txID)
	o.Require().NoError(err)
	msg.Amount = sdk.NewCoins(sdk.NewCoin("ujolt", sdk.NewInt(2000000)))
	item, err := oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, poolInfo, memo, &msg, txIDByte)
	o.Require().NoError(err)
	oc.AddItem(item)

	oc.RetryOutboundReq.Range(func(key, value any) bool {
		item := value.(*common2.OutBoundReq)
		o.Require().Equal(item.Coin.Amount, sdk.NewInt(1916667))
		oc.RetryOutboundReq.Delete(key)
		return true
	})

	// test native token
	txID = "d03fb2b6ae7690afa037ecc44a24e67de2676777b75efcbd1a9bea9e6cc16581"
	txIDByte, err = hex.DecodeString(txID)
	o.Require().NoError(err)
	item, err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, poolInfo, memo, &msg, txIDByte)
	o.Require().NoError(err)
	oc.AddItem(item)

	oc.RetryOutboundReq.Range(func(key, value any) bool {
		item := value.(*common2.OutBoundReq)
		o.Require().Equal(item.Coin.Amount.String(), sdk.NewInt(1916667).String())
		return true
	})
}

func (o OutBoundTestSuite) TestProcessToken() {
	accs, err := generateRandomPrivKey(4)
	o.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"native", "testAddr2"}, []string{"abnb", "ujolt"}, []string{"BSC", "BSC"})

	rp := common2.NewRetryPools()
	o.Require().NoError(err)
	oc, err := NewJoltifyBridge(o.network.Validators[0].RPCAddress, o.network.Validators[0].RPCAddress, o.network.Validators[0].ClientCtx, &tss, tl, rp)
	o.Require().NoError(err)
	defer func() {
		err2 := oc.TerminateBridge()
		if err2 != nil {
			oc.logger.Error().Err(err2).Msgf("fail to terminate the bridge")
		}
	}()

	msg := banktypes.MsgSend{}
	txID := hex.EncodeToString([]byte("testTxID"))
	blockHeight := 100

	memo := common2.BridgeMemo{
		Dest: accs[2].commAddr.String(),
	}

	poolInfo := &common2.PoolInfo{
		Pk:         accs[0].pk,
		CosAddress: accs[0].joltAddr,
		EthAddress: accs[0].commAddr,
	}

	amount, ok := sdk.NewIntFromString("1200000")
	o.Require().True(ok)
	coin1 := sdk.NewCoin("ujolt", amount)
	memo.ChainType = "BSC"
	msg.Amount = sdk.Coins{coin1}

	item, err := oc.processOutBoundRequest(&msg, txID, int64(blockHeight), poolInfo, memo)
	oc.AddItem(item)
	o.Require().NoError(err)

	r := oc.PopItem(1, "BSC")
	o.Require().Len(r, 1)
	tokens := r[0].Coin

	oc.CosHandler.GrpcLock.Lock()
	price, err := QueryTokenPrice(oc.CosHandler.GrpcClient, "", "ujolt")
	o.Require().NoError(err)
	oc.CosHandler.GrpcLock.Unlock()
	val := new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(6)), nil)
	fee := oc.FeeModule["BSC"].Floor.Mul(sdk.NewDecFromBigInt(val)).Quo(price).RoundInt()

	expected := amount.Sub(fee)

	delta := sdk.NewInt(2)
	o.Require().True(expected.Sub(tokens.Amount).Abs().LT(delta))

	// we test the native token and too small amount
	amount, ok = sdk.NewIntFromString("1200000")
	o.Require().True(ok)
	coin2 := sdk.NewCoin("abnb", amount)
	memo.ChainType = "BSC"
	msg.Amount = sdk.Coins{coin2}
	item, err = oc.processOutBoundRequest(&msg, txID, int64(blockHeight), poolInfo, memo)
	oc.AddItem(item)
	o.Require().NoError(err)

	r = oc.PopItem(1, "BSC")
	o.Require().Len(r, 0)
	amount, ok = sdk.NewIntFromString("1200000000000000000")
	o.Require().True(ok)
	coin2.Amount = amount
	msg.Amount = sdk.Coins{coin2}
	item, err = oc.processOutBoundRequest(&msg, txID, int64(blockHeight), poolInfo, memo)
	oc.AddItem(item)
	o.Require().NoError(err)
	r = oc.PopItem(1, "BSC")

	o.Require().Len(r, 1)

	tokens = r[0].Coin

	oc.CosHandler.GrpcLock.Lock()
	price, err = QueryTokenPrice(oc.CosHandler.GrpcClient, "", "abnb")
	o.Require().NoError(err)
	oc.CosHandler.GrpcLock.Unlock()
	val = new(big.Int).Exp(big.NewInt(10), new(big.Int).Abs(big.NewInt(18)), nil)
	fee = oc.FeeModule["BSC"].Floor.Mul(sdk.NewDecFromBigInt(val)).Quo(price).RoundInt()

	expected = amount.Sub(fee)

	delta = sdk.NewInt(2)
	o.Require().True(expected.Sub(tokens.Amount).Abs().LT(delta))

	// we process the token not on our list
	coin3 := sdk.NewCoin("sbnb", amount)
	memo.ChainType = "BSC"
	msg.Amount = sdk.Coins{coin3}
	item, err = oc.processOutBoundRequest(&msg, txID, int64(blockHeight), poolInfo, memo)
	oc.AddItem(item)
	o.Require().Error(err)
}

func TestTxOutBound(t *testing.T) {
	suite.Run(t, new(OutBoundTestSuite))
}

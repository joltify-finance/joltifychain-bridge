package oppybridge

import (
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	grpc1 "github.com/gogo/protobuf/grpc"
	common2 "gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppychain/testutil/network"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
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
	cfg.ChainID = config.ChainID
	o.cfg = cfg
	o.validatorky = keyring.NewInMemory()
	// now we put the mock pool list in the test
	state := vaulttypes.GenesisState{}
	stateStaking := stakingtypes.GenesisState{}

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
			Nodes:      nodes,
		}
		state.CreatePoolList = append(state.CreatePoolList, &vaulttypes.CreatePool{BlockHeight: strconv.Itoa(i), Validators: validators, Proposal: []*vaulttypes.PoolProposal{&pro}})
	}
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

	_, err = o.network.WaitForHeight(1)
	o.Require().Nil(err)
	o.grpc = o.network.Validators[0].ClientCtx
	o.queryClient = tmservice.NewServiceClient(o.network.Validators[0].ClientCtx)
}

type account struct {
	sk       *secp256k1.PrivKey
	pk       string
	oppyAddr sdk.AccAddress
	commAddr common.Address
}

func generateRandomPrivKey(n int) ([]account, error) {
	randomAccounts := make([]account, n)
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
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	o.Require().NoError(err)
	oc, err := NewOppyBridge(o.network.Validators[0].APIAddress, o.network.Validators[0].RPCAddress, &tss, tl)
	o.Require().NoError(err)
	defer func() {
		err := oc.TerminateBridge()
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()
	//
	key, _, err := oc.Keyring.NewMnemonic("pooltester1", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	o.Require().NoError(err)

	key2, _, err := oc.Keyring.NewMnemonic("pooltester2", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	o.Require().NoError(err)

	key3, _, err := oc.Keyring.NewMnemonic("pooltester3", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	o.Require().NoError(err)

	cospk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, key.GetPubKey()) // nolint

	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: cospk,
			PoolAddr:   accs[0].oppyAddr,
		},
	}

	oc.UpdatePool(&poolInfo)
	pubkeyStr := key.GetPubKey().Address().String()
	pk, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	pools := oc.GetPool()
	o.Require().Nil(pools[0])
	addr2 := pools[1].OppyAddress
	o.Require().True(pk.Equals(addr2))
	// now we add another pool
	cospk = legacybech32.MustMarshalPubKey(legacybech32.AccPK, key2.GetPubKey()) // nolint

	poolInfo = vaulttypes.PoolInfo{
		BlockHeight: "101",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: cospk,
			PoolAddr:   accs[0].oppyAddr,
		},
	}
	oc.UpdatePool(&poolInfo)
	pools = oc.GetPool()
	pubkeyStr = key2.GetPubKey().Address().String()
	pk2, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	o.Require().True(pk.Equals(pools[0].OppyAddress))

	pool1 := oc.lastTwoPools[1].OppyAddress
	o.Require().True(pk2.Equals(pool1))

	// now we add another pool and pop out the firt one

	cospk = legacybech32.MustMarshalPubKey(legacybech32.AccPK, key3.GetPubKey()) // nolint

	poolInfo = vaulttypes.PoolInfo{
		BlockHeight: "102",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: cospk,
			PoolAddr:   accs[0].oppyAddr,
		},
	}
	oc.UpdatePool(&poolInfo)
	pools = oc.GetPool()

	pubkeyStr = key3.GetPubKey().Address().String()
	pk3, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	o.Require().True(pk3.Equals(pools[1].OppyAddress))

	time.Sleep(time.Second)
}

func (o OutBoundTestSuite) TestOutBoundReq() {
	accs, err := generateRandomPrivKey(2)

	o.Require().NoError(err)
	boundReq := common2.NewOutboundReq("testID", accs[0].commAddr, accs[1].commAddr, sdk.NewCoin("JUSD", sdk.NewInt(1)), AddrJUSD, 101, nil, nil)
	boundReq.SetItemNonce(accs[1].commAddr, 100)
	a, b, _, amount, h := boundReq.GetOutBoundInfo()
	o.Require().Equal(a.String(), accs[0].commAddr.String())
	o.Require().Equal(b.String(), accs[1].commAddr.String())
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
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr", "native"}, []string{"testToken", config.OutBoundDenomFee})
	o.Require().NoError(err)
	oc, err := NewOppyBridge(o.network.Validators[0].RPCAddress, o.network.Validators[0].RPCAddress, &tss, tl)
	o.Require().NoError(err)
	defer func() {
		err2 := oc.TerminateBridge()
		if err2 != nil {
			oc.logger.Error().Err(err2).Msgf("fail to terminate the bridge")
		}
	}()

	// we need to add this as it seems the rpcaddress is incorrect
	oc.GrpcClient = o.network.Validators[0].ClientCtx
	baseBlockHeight := int64(100)
	msg := banktypes.MsgSend{}
	memo := common2.BridgeMemo{
		Dest: accs[0].commAddr.String(),
	}

	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "empty address string is not allowed")

	msg.FromAddress = o.network.Validators[0].Address.String()
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "empty address string is not allowed")

	ret := oc.CheckWhetherAlreadyExist(o.grpc, "testindex")
	o.Require().True(ret)

	msg.ToAddress = accs[3].oppyAddr.String()
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "not a top up message to the pool")

	msg.ToAddress = accs[1].oppyAddr.String()
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "incorrect msg format")

	fee := sdk.NewCoin(config.OutBoundDenomFee, sdk.NewInt(100))
	coin2 := sdk.NewCoin("invalidToken", sdk.NewInt(1))
	coin3 := sdk.NewCoin("invalidToken2", sdk.NewInt(100))
	topUptoken := sdk.NewCoin("testToken", sdk.NewInt(100))

	msg.Amount = sdk.Coins{fee, coin2, coin3}
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "incorrect msg format")

	msg.Amount = sdk.Coins{fee, coin2}
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, []byte("msg1"))
	o.Require().EqualError(err, "fail to process the outbound erc20 request")

	// test ERC20 token
	txID := "5d3a86ed8923343038a6c847d6b71c8dfe8e507fdda748223a28e860756f6afe"
	txIDByte, err := hex.DecodeString(txID)
	o.Require().NoError(err)
	msg.Amount = sdk.Coins{fee, topUptoken}
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, txIDByte)
	o.Require().NoError(err)

	// in reality, we will not have two tx with same txID
	msg.Amount = sdk.Coins{fee}
	memo.TopupID = txID
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, []byte("any"))
	o.Require().NoError(err)

	dat, ok := oc.pendingTx.Load(txID)
	o.Require().True(ok)
	FeeWeGet := dat.(*OutboundTx).Fee.Amount
	o.Require().Equal(FeeWeGet, sdk.NewInt(200))

	expectedFee := oc.calculateGas()

	delta := expectedFee.SubAmount(FeeWeGet)
	memo.TopupID = txID
	msg.Amount = []sdk.Coin{delta}
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, []byte("any"))
	o.Require().NoError(err)
	_, ok = oc.pendingTx.Load(txID)
	o.Require().False(ok)

	oc.RetryOutboundReq.Range(func(key, value any) bool {
		item := value.(*common2.OutBoundReq)
		o.Require().Equal(item.Coin.Amount, sdk.NewInt(100))
		oc.RetryOutboundReq.Delete(key)
		return true
	})

	// test native token
	txID = "d03fb2b6ae7690afa037ecc44a24e67de2676777b75efcbd1a9bea9e6cc16581"
	txIDByte, err = hex.DecodeString(txID)
	o.Require().NoError(err)
	msg.Amount = sdk.Coins{fee}
	memo.TopupID = ""
	err = oc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].oppyAddr, accs[2].oppyAddr}, accs[3].commAddr, memo, &msg, txIDByte)
	o.Require().NoError(err)

	_, ok = oc.pendingTx.Load(txID)
	o.Require().False(ok)

	oc.RetryOutboundReq.Range(func(key, value any) bool {
		item := value.(*common2.OutBoundReq)
		o.Require().Equal(item.Coin.Amount.String(), sdk.NewInt(0).String())
		return true
	})
}

func (o OutBoundTestSuite) TestProcessErc20Token() {
	accs, err := generateRandomPrivKey(4)
	o.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"native", "testAddr2"}, []string{config.OutBoundDenomFee, "testToken"})
	o.Require().NoError(err)
	oc, err := NewOppyBridge(o.network.Validators[0].RPCAddress, o.network.Validators[0].RPCAddress, &tss, tl)
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
	receiverAddr := accs[0].commAddr

	memo := common2.BridgeMemo{
		Dest: accs[2].commAddr.String(),
	}

	coin1 := sdk.NewCoin("testToken", sdk.NewInt(100))
	coinFee := sdk.NewCoin(config.OutBoundDenomFee, sdk.NewInt(100))
	invalidFee := sdk.NewCoin("invalidFee", sdk.NewInt(100000000000000000))
	msg.Amount = sdk.Coins{coin1, invalidFee}
	err = oc.processErc20Request(&msg, txID, int64(blockHeight), receiverAddr, memo)
	o.Require().EqualError(err, "invalid fee pair")

	coinInvalid := sdk.NewCoin("invalid", sdk.NewInt(12))
	msg.Amount = sdk.Coins{coinInvalid, coinFee}
	err = oc.processErc20Request(&msg, txID, int64(blockHeight), receiverAddr, memo)
	o.Require().EqualError(err, "invalid fee pair")

	txIDNotEnoughFee := hex.EncodeToString([]byte("txnotenoughfee"))
	msg.Amount = sdk.Coins{coin1, coinFee}
	err = oc.processErc20Request(&msg, txIDNotEnoughFee, int64(blockHeight), receiverAddr, memo)
	o.Require().NoError(err)

	msg.Amount = sdk.Coins{coinFee, coin1}
	err = oc.processErc20Request(&msg, txIDNotEnoughFee, int64(blockHeight), receiverAddr, memo)
	o.Require().NoError(err)

	val, ok := oc.pendingTx.Load(txIDNotEnoughFee)
	o.Require().True(ok)
	o.Require().Equal(val.(*OutboundTx).OutReceiverAddress.String(), accs[2].commAddr.String())
	o.Require().True(val.(*OutboundTx).Token.Amount.Equal(coinFee.Amount))

	memo = common2.BridgeMemo{
		Dest:    accs[0].commAddr.String(),
		TopupID: txIDNotEnoughFee + "invalid",
	}

	msg.Amount = []sdk.Coin{coin1}
	err = oc.processTopUpRequest(&msg, int64(101), receiverAddr, memo)
	o.Require().EqualError(err, "token is not on our token list or not fee demon")

	// the pending tx does not exit
	msg.Amount = []sdk.Coin{coin1}
	memo.TopupID = txIDNotEnoughFee
	err = oc.processTopUpRequest(&msg, int64(101), receiverAddr, memo)
	o.Require().NotNil(err)
	val, ok = oc.pendingTx.Load(txIDNotEnoughFee)
	o.Require().True(ok)
	o.Require().Equal(val.(*OutboundTx).OutReceiverAddress.String(), accs[2].commAddr.String())
	o.Require().True(val.(*OutboundTx).Token.Amount.Equal(coinFee.Amount.MulRaw(1)))

	msg.Amount = []sdk.Coin{coinFee}
	err = oc.processTopUpRequest(&msg, int64(101), receiverAddr, memo)
	o.Require().NoError(err)
	val, ok = oc.pendingTx.Load(txIDNotEnoughFee)
	o.Require().True(ok)
	o.Require().Equal(val.(*OutboundTx).OutReceiverAddress.String(), accs[2].commAddr.String())

	o.Require().True(val.(*OutboundTx).Fee.Amount.Equal(coinFee.Amount.MulRaw(2)))

	// now we pay enough fee
	expectedFee := oc.calculateGas()

	delta := expectedFee.Sub(val.(*OutboundTx).Fee)
	msg.Amount = []sdk.Coin{delta}
	err = oc.processTopUpRequest(&msg, int64(101), receiverAddr, memo)
	o.Require().NoError(err)

	_, ok = oc.pendingTx.Load(txIDNotEnoughFee)
	o.Require().False(ok)
	items := oc.PopItem(1)

	oc.pendingTx.Range(func(key, value any) bool {
		panic("it should be empty")
	})

	o.Require().Equal(items[0].TxID, txIDNotEnoughFee)
	o.Require().Equal(items[0].OutReceiverAddress.String(), accs[2].commAddr.String())
	o.Require().Equal(items[0].Coin.Denom, coin1.Denom)
	o.Require().Equal(items[0].Coin.Amount.Int64(), int64(100))
}

func (o OutBoundTestSuite) TestProcessNativeToken() {
	accs, err := generateRandomPrivKey(4)
	o.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr", "native"}, []string{"testToken", "abnb"})
	o.Require().NoError(err)
	oc, err := NewOppyBridge(o.network.Validators[0].RPCAddress, o.network.Validators[0].RPCAddress, &tss, tl)
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
	receiverAddr := accs[0].commAddr

	memo := common2.BridgeMemo{
		Dest: accs[2].commAddr.String(),
	}

	coin3 := sdk.NewCoin("invalid", sdk.NewInt(100))
	msg.Amount = sdk.Coins{coin3}
	err = oc.processNativeRequest(&msg, txID, int64(blockHeight), receiverAddr, memo)
	o.Require().EqualError(err, "token is not on our token list")

	coin4 := sdk.NewCoin("abnb", sdk.NewInt(100))
	msg.Amount = sdk.Coins{coin4}
	err = oc.processNativeRequest(&msg, txID, int64(blockHeight), receiverAddr, memo)
	o.Require().NoError(err)

	expectedFee := oc.calculateGas()

	coin4 = sdk.NewCoin("abnb", expectedFee.Amount)
	msg.Amount = sdk.Coins{coin4}
	err = oc.processNativeRequest(&msg, txID, int64(blockHeight), receiverAddr, memo)
	o.Require().NoError(err)

	counter := 0
	oc.pendingTx.Range(func(key, value any) bool {
		counter++
		return true
	})

	o.Require().Equal(counter, 0)
}

func (o OutBoundTestSuite) TestProcessNativeTokenTopUp() {
	accs, err := generateRandomPrivKey(4)
	o.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr", "native"}, []string{"testToken", "abnb"})
	o.Require().NoError(err)
	oc, err := NewOppyBridge(o.network.Validators[0].RPCAddress, o.network.Validators[0].RPCAddress, &tss, tl)
	o.Require().NoError(err)
	defer func() {
		err2 := oc.TerminateBridge()
		if err2 != nil {
			oc.logger.Error().Err(err2).Msgf("fail to terminate the bridge")
		}
	}()

	msg := banktypes.MsgSend{}
	txIDNotEnoughFee := hex.EncodeToString([]byte("testTxID"))
	blockHeight := 100
	receiverAddr := accs[0].commAddr

	memo := common2.BridgeMemo{
		Dest: accs[2].commAddr.String(),
	}

	fee := sdk.NewCoin("abnb", sdk.NewInt(100))
	msg.Amount = sdk.Coins{fee}

	err = oc.processNativeRequest(&msg, txIDNotEnoughFee, int64(blockHeight), receiverAddr, memo)
	o.Require().NoError(err)

	//we drop the native token if it is not enough for the fee
	_, ok := oc.pendingTx.Load(txIDNotEnoughFee)
	o.Require().False(ok)

	oc.pendingTx.Range(func(key, value any) bool {
		panic("it should be empty")
	})

}

func TestTxOutBound(t *testing.T) {
	suite.Run(t, new(OutBoundTestSuite))
}

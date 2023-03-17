package cossubmit

import (
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/joltify-finance/joltify_lending/testutil/network"
	pricefeedtypes "github.com/joltify-finance/joltify_lending/x/third_party/pricefeed/types"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
)

type helperTestSuite struct {
	suite.Suite
	cfg          network.Config
	network      *network.Network
	validatorkey keyring.Keyring
	queryClient  tmservice.ServiceClient
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

func genNValidator(n int, validatorky keyring.Keyring) ([]stakingtypes.Validator, error) {
	var validators []stakingtypes.Validator
	var uid string
	for i := 0; i < n; i++ {
		if i == 0 {
			uid = "operator"
		} else {
			uid = "o" + strconv.Itoa(i)
		}
		info, _, err := validatorky.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
		if err != nil {
			return nil, err
		}

		operator, err := sdk.ValAddressFromHex(info.GetPubKey().Address().String())
		if err != nil {
			return nil, err
		}
		desc := stakingtypes.NewDescription("tester", "testId", "www.test.com", "aaa", "aaa")
		testValidator, err := stakingtypes.NewValidator(operator, info.GetPubKey(), desc)
		if err != nil {
			return nil, err
		}
		validators = append(validators, testValidator)
	}
	return validators, nil
}

func (h *helperTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.BondDenom = "stake"
	cfg.BondedTokens = sdk.NewInt(10000000000000000)
	cfg.StakingTokens = sdk.NewInt(100000000000000000)
	h.cfg = cfg
	h.validatorkey = keyring.NewInMemory()
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
	h.Require().NoError(err)
	cfg.GenesisState[pricefeedtypes.ModuleName] = bufPriceFeed

	h.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[vaulttypes.ModuleName], &state))
	h.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateStaking))

	validators, err := genNValidator(3, h.validatorkey)
	h.Require().NoError(err)
	for i := 1; i < 5; i++ {
		randPoolSk := ed25519.GenPrivKey()
		poolPubKey, err := legacybech32.MarshalPubKey(legacybech32.AccPK, randPoolSk.PubKey()) // nolint
		h.Require().NoError(err)

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
	h.Require().NoError(err)
	cfg.GenesisState[vaulttypes.ModuleName] = buf

	var stateVault stakingtypes.GenesisState
	h.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateVault))
	stateVault.Params.MaxValidators = 3
	state.Params.BlockChurnInterval = 1
	buf, err = cfg.Codec.MarshalJSON(&stateVault)
	h.Require().NoError(err)
	cfg.GenesisState[stakingtypes.ModuleName] = buf

	stateBank := banktypes.GenesisState{}
	require.NoError(h.T(), cfg.Codec.UnmarshalJSON(cfg.GenesisState[banktypes.ModuleName], &stateBank))

	stateBank.Balances = []banktypes.Balance{{Address: "jolt1txtsnx4gr4effr8542778fsxc20j5vzqxet7t0", Coins: sdk.Coins{sdk.NewCoin("stake", sdk.NewInt(100000))}}}
	bankBuf, err := cfg.Codec.MarshalJSON(&stateBank)
	require.NoError(h.T(), err)
	cfg.GenesisState[banktypes.ModuleName] = bankBuf

	h.network = network.New(h.T(), cfg)

	h.Require().NotNil(h.network)

	_, err = h.network.WaitForHeight(1)
	h.Require().Nil(err)
	h.queryClient = tmservice.NewServiceClient(h.network.Validators[0].ClientCtx)
}

func (h *helperTestSuite) TestWaitandSend() {
	handler, err := NewCosOperations(h.network.Validators[0].RPCAddress, "", h.network.Validators[0].ClientCtx, nil, "test", nil)
	h.Require().NoError(err)
	handler.GrpcClient = h.network.Validators[0].ClientCtx
	info, _ := h.network.Validators[0].ClientCtx.Keyring.Key("node0")
	pk := info.GetPubKey()
	pkstr := legacybech32.MustMarshalPubKey(legacybech32.AccPK, pk) // nolint
	valAddr, err := misc.PoolPubKeyToJoltifyAddress(pkstr)
	h.Require().NoError(err)
	acc, err := bcommon.QueryAccount(handler.GrpcClient, valAddr.String(), "")
	h.Require().NoError(err)
	acc.GetSequence()

	err = handler.waitAndSend(handler.GrpcClient, valAddr.String(), acc.GetSequence())
	h.Require().NoError(err)

	err = handler.waitAndSend(handler.GrpcClient, valAddr.String(), acc.GetSequence()-1)
	h.Require().Error(err, "already passed")

	err = handler.waitAndSend(handler.GrpcClient, "mock", acc.GetSequence()-1)
	h.Require().Error(err, "invalid Account query")
}

func (h *helperTestSuite) TestBatchComposeAndSend() {
	accs, err := generateRandomPrivKey(3)
	h.Require().NoError(err)
	tss := TssMock{
		accs[0].sk,
		h.network.Validators[0].ClientCtx.Keyring,
		true,
		true,
	}

	handler, err := NewCosOperations(h.network.Validators[0].RPCAddress, "", h.network.Validators[0].ClientCtx, nil, "test", &tss)
	h.Require().NoError(err)
	handler.GrpcClient = h.network.Validators[0].ClientCtx

	info, _ := h.network.Validators[0].ClientCtx.Keyring.Key("node0")
	pk := info.GetPubKey()
	pkstr := legacybech32.MustMarshalPubKey(legacybech32.AccPK, pk) // nolint
	valAddr, err := misc.PoolPubKeyToJoltifyAddress(pkstr)
	h.Require().NoError(err)

	operatorInfo, err := h.validatorkey.Key("operator")
	h.Require().NoError(err)

	signMsg := tssclient.TssSignigMsg{
		Pk:          pkstr,
		Signers:     nil,
		BlockHeight: 10,
		Version:     tssclient.TssVersion,
	}

	acc, err := bcommon.QueryAccount(handler.GrpcClient, valAddr.String(), "")
	h.Require().NoError(err)

	send := banktypes.NewMsgSend(valAddr, operatorInfo.GetAddress(), sdk.Coins{sdk.NewCoin("stake", sdk.NewInt(100))})
	_, err = handler.BatchComposeAndSend(handler.GrpcClient, []sdk.Msg{send}, acc.GetSequence(), acc.GetAccountNumber(), &signMsg, valAddr.String(), []string{"mock"})
	h.Require().Error(err, "operator.info: key not found")

	handler.Keyring = h.validatorkey
	_, err = handler.BatchComposeAndSend(handler.GrpcClient, []sdk.Msg{send}, acc.GetSequence(), acc.GetAccountNumber(), &signMsg, valAddr.String(), []string{"mock"})
	h.Require().NoError(err)
}

func (h *helperTestSuite) TestOppyChainBridge_CheckWhetherAlreadyExist() {
	oc, err := NewCosOperations("", "", h.network.Validators[0].ClientCtx, nil, "tset", nil)
	h.Require().NoError(err)
	oc.GrpcClient = h.network.Validators[0].ClientCtx
	ret := oc.CheckWhetherIssueTokenAlreadyExist(oc.GrpcClient, "testindex")
	h.Require().True(ret)

	ret = oc.CheckWhetherIssueTokenAlreadyExist(oc.GrpcClient, "testindexnoexist")
	h.Require().False(ret)
}

func (h *helperTestSuite) TestCheckTxStatus() {
	oc, err := NewCosOperations("", "", h.network.Validators[0].ClientCtx, nil, "tset", nil)
	h.Require().NoError(err)
	oc.GrpcClient = h.network.Validators[0].ClientCtx
	err = oc.CheckIssueTokenTxStatus(oc.GrpcClient, "testindex", 1)
	h.Require().NoError(err)
	err = oc.CheckIssueTokenTxStatus(oc.GrpcClient, "testindexnoexist", 1)
	h.Require().Error(err)
}

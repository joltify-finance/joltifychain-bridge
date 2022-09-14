package oppybridge

import (
	"context"
	"encoding/hex"
	"go.uber.org/atomic"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	grpc1 "github.com/gogo/protobuf/grpc"
	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"
	"gitlab.com/oppy-finance/oppy-bridge/validators"
	"gitlab.com/oppy-finance/oppychain/testutil/network"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

type SubmitOutBoundTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
	grpc        grpc1.ClientConn
}

func (v *SubmitOutBoundTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.BondDenom = "stake"
	cfg.MinGasPrices = "0stake"
	cfg.ChainID = config.ChainID
	v.cfg = cfg
	v.validatorky = keyring.NewInMemory()
	// now we put the mock pool list in the test
	state := vaulttypes.GenesisState{}
	stateStaking := stakingtypes.GenesisState{}

	v.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[vaulttypes.ModuleName], &state))
	v.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateStaking))

	testToken := vaulttypes.IssueToken{
		Index: "testindex",
	}
	state.IssueTokenList = append(state.IssueTokenList, &testToken)

	// add the validators
	var allV []*vaulttypes.Validator
	for i := 0; i < 4; i++ {
		sk := ed25519.GenPrivKey()
		v := vaulttypes.Validator{Pubkey: sk.PubKey().Bytes(), Power: 10}
		allV = append(allV, &v)
	}

	state.ValidatorinfoList = append(state.ValidatorinfoList, &vaulttypes.Validators{AllValidators: allV, Height: 20})
	state.ValidatorinfoList = append(state.ValidatorinfoList, &vaulttypes.Validators{AllValidators: allV, Height: 40})
	state.ValidatorinfoList = append(state.ValidatorinfoList, &vaulttypes.Validators{AllValidators: allV, Height: 60})
	state.ValidatorinfoList = append(state.ValidatorinfoList, &vaulttypes.Validators{AllValidators: allV, Height: 80})

	buf, err := cfg.Codec.MarshalJSON(&state)
	v.Require().NoError(err)
	cfg.GenesisState[vaulttypes.ModuleName] = buf

	var stateVault stakingtypes.GenesisState
	v.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateVault))
	stateVault.Params.MaxValidators = 3
	state.Params.BlockChurnInterval = 1
	buf, err = cfg.Codec.MarshalJSON(&stateVault)
	v.Require().NoError(err)
	cfg.GenesisState[stakingtypes.ModuleName] = buf

	v.network = network.New(v.T(), cfg)

	v.Require().NotNil(v.network)

	_, err = v.network.WaitForHeight(1)
	v.Require().Nil(err)

	sk, err := v.network.Validators[0].ClientCtx.Keyring.ExportPrivKeyArmor("node0", "12345678")
	v.Require().NoError(err)
	err = v.validatorky.ImportPrivKey("operator", sk, "12345678")
	v.Require().NoError(err)
	v.grpc = v.network.Validators[0].ClientCtx
	v.queryClient = tmservice.NewServiceClient(v.network.Validators[0].ClientCtx)
}

func (s SubmitOutBoundTestSuite) TestSubmitOutboundTx() {
	accs, err := generateRandomPrivKey(2)
	s.Require().NoError(err)
	tss := TssMock{
		accs[0].sk,
		// nil,
		s.network.Validators[0].ClientCtx.Keyring,
		// m.network.Validators[0].ClientCtx.Keyring,
		true,
		true,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	s.Require().NoError(err)
	oc, err := NewOppyBridge(s.network.Validators[0].APIAddress, s.network.Validators[0].RPCAddress, &tss, tl)
	s.Require().NoError(err)
	oc.Keyring = s.validatorky

	// we need to add this as it seems the rpcaddress is incorrect
	oc.GrpcClient = s.network.Validators[0].ClientCtx
	defer func() {
		err := oc.TerminateBridge()
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()

	_, err = s.network.WaitForHeightWithTimeout(11, time.Minute)
	s.Require().NoError(err)

	oc.validatorSet = validators.NewValidator()
	wg := sync.WaitGroup{}
	inkeygen := atomic.NewBool(true)
	err = oc.HandleUpdateValidators(20, &wg, inkeygen)
	s.Require().NoError(err)
	info, _ := s.network.Validators[0].ClientCtx.Keyring.Key("node0")
	pk := info.GetPubKey()
	pkstr := legacybech32.MustMarshalPubKey(legacybech32.AccPK, pk) // nolint
	valAddr, err := misc.PoolPubKeyToOppyAddress(pkstr)
	s.Require().NoError(err)

	acc, err := queryAccount(s.grpc, valAddr.String(), "")
	s.Require().NoError(err)

	operatorInfo, _ := oc.Keyring.Key("operator")

	send := banktypes.NewMsgSend(valAddr, operatorInfo.GetAddress(), sdk.Coins{sdk.NewCoin("stake", sdk.NewInt(100))})

	txBuilder, err := Gensigntx(oc, []sdk.Msg{send}, info, acc.GetAccountNumber(), acc.GetSequence(), s.network.Validators[0].ClientCtx.Keyring)
	s.Require().NoError(err)
	txBytes, err := oc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	s.Require().NoError(err)
	ret, _, err := oc.BroadcastTx(context.Background(), s.grpc, txBytes, false)
	s.Require().NoError(err)
	s.Require().True(ret)

	req := common.OutBoundReq{
		TxID:               hex.EncodeToString([]byte("testreq")),
		OutReceiverAddress: accs[0].commAddr,
	}
	s.Require().NoError(err)
	err = oc.SubmitOutboundTx(s.grpc, info, req.Hash().Hex(), 10, hex.EncodeToString([]byte("testpubtx")), sdk.NewCoins(sdk.NewCoin("a", sdk.NewInt(32))))
	s.Require().NoError(err)
	_, err = oc.GetPubChainSubmittedTx(req)
	s.Require().NoError(err)
}

func TestSubmitOutBound(t *testing.T) {
	suite.Run(t, new(SubmitOutBoundTestSuite))
}

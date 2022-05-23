package joltifybridge

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain/testutil/network"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

type SubmitOutBoundTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
}

func (v *SubmitOutBoundTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.MinGasPrices = "0stake"
	cfg.ChainID = config.ChainID
	v.cfg = cfg
	v.validatorky = keyring.NewInMemory()
	// now we put the mock pool list in the test
	state := vaulttypes.GenesisState{}
	stateStaking := stakingtypes.GenesisState{}

	v.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[vaulttypes.ModuleName], &state))
	v.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateStaking))

	validators, err := genNValidator(3, v.validatorky)
	v.Require().NoError(err)
	for i := 1; i < 5; i++ {
		randPoolSk := ed25519.GenPrivKey()
		poolPubKey, err := legacybech32.MarshalPubKey(legacybech32.AccPK, randPoolSk.PubKey()) // nolint
		v.Require().NoError(err)

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
	jc, err := NewJoltifyBridge(s.network.Validators[0].APIAddress, s.network.Validators[0].RPCAddress, &tss)
	s.Require().NoError(err)
	jc.Keyring = s.validatorky

	// we need to add this as it seems the rpcaddress is incorrect
	jc.grpcClient = s.network.Validators[0].ClientCtx
	defer func() {
		err := jc.TerminateBridge()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()

	_, err = s.network.WaitForHeightWithTimeout(10, time.Second*30)
	s.Require().NoError(err)

	info, _ := s.network.Validators[0].ClientCtx.Keyring.Key("node0")
	pk := info.GetPubKey()
	pkstr := legacybech32.MustMarshalPubKey(legacybech32.AccPK, pk) // nolint
	valAddr, err := misc.PoolPubKeyToJoltAddress(pkstr)
	s.Require().NoError(err)

	acc, err := queryAccount(valAddr.String(), jc.grpcClient)
	s.Require().NoError(err)

	operatorInfo, _ := jc.Keyring.Key("operator")

	send := banktypes.NewMsgSend(valAddr, operatorInfo.GetAddress(), sdk.Coins{sdk.NewCoin("stake", sdk.NewInt(100))})

	//txBuilder, err := jc.genSendTx([]sdk.Msg{send}, acc.GetSequence(), acc.GetAccountNumber(), 200000, &signMsg)
	txBuilder, err := Gensigntx(jc, []sdk.Msg{send}, info, acc.GetAccountNumber(), acc.GetSequence(), s.network.Validators[0].ClientCtx.Keyring)
	s.Require().NoError(err)
	txBytes, err := jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	s.Require().NoError(err)
	ret, _, err := jc.BroadcastTx(context.Background(), txBytes, false)
	s.Require().NoError(err)
	s.Require().True(ret)

	req := common.OutBoundReq{
		TxID:               "testreq",
		OutReceiverAddress: accs[0].commAddr,
		OriginalHeight:     5,
	}
	s.Require().NoError(err)
	err = jc.SubmitOutboundTx(req.Hash().Hex(), 10, "testpubtx")
	s.Require().NoError(err)
	_, err = jc.GetPubChainSubmittedTx(req)
	//since the operator is not the validator, so its sbumition is expected to be rejected,
	// and here it returns record not found
	s.Require().Equal("rpc error: code = InvalidArgument desc = rpc error: code = InvalidArgument desc = not found: invalid request", err.Error())
}

func TestSubmitOutBound(t *testing.T) {
	suite.Run(t, new(SubmitOutBoundTestSuite))
}

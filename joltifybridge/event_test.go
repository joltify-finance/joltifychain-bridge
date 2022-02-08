package joltifybridge

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/suite"
	"github.com/tendermint/tendermint/crypto/secp256k1"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain/testutil/network"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

type EventTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
}

func (v *EventTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
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

func (e EventTestSuite) TestSubscribe() {
	accs, err := generateRandomPrivKey(2)
	e.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}
	jc, err := NewJoltifyBridge(e.network.Validators[0].APIAddress, e.network.Validators[0].RPCAddress, &tss)
	e.Require().NoError(err)
	defer func() {
		err := jc.TerminateBridge()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()
	query := "tm.event = 'NewBlock'"
	eventChain, err := jc.AddSubscribe(context.Background(), query)
	e.Require().NoError(err)
	data := <-eventChain
	e.T().Logf("new block event test %v", data.Events)
}

func (e EventTestSuite) TestHandleUpdateEvent() {
	accs, err := generateRandomPrivKey(2)
	e.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		false,
		true,
	}
	jc, err := NewJoltifyBridge(e.network.Validators[0].APIAddress, e.network.Validators[0].RPCAddress, &tss)
	e.Require().NoError(err)
	defer func() {
		err := jc.TerminateBridge()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()
	jc.grpcClient = e.network.Validators[0].ClientCtx
	err = jc.InitValidators(e.network.Validators[0].APIAddress)
	e.Require().NoError(err)
	data := base64.StdEncoding.EncodeToString(e.network.Validators[0].PubKey.Bytes())
	jc.myValidatorInfo.Result.ValidatorInfo.PubKey.Value = data

	sk := secp256k1.GenPrivKey()
	remoteValidator := tmtypes.NewValidator(sk.PubKey(), 100)
	err = jc.HandleUpdateValidators([]*tmtypes.Validator{remoteValidator}, 10)
	e.Require().NoError(err)
	e.Require().Equal(len(jc.msgSendCache), 0)
	tss.keygenSuccess = true

	jc.Keyring = e.validatorky
	info, err := jc.Keyring.Key("operator")
	e.Require().NoError(err)
	errorMSg := fmt.Sprintf("rpc error: code = NotFound desc = rpc error: code = NotFound desc = account %v not found: key not found", info.GetAddress().String())
	err = jc.HandleUpdateValidators([]*tmtypes.Validator{remoteValidator}, 10)
	e.Require().EqualError(err, errorMSg)
}

func TestEvent(t *testing.T) {
	suite.Run(t, new(EventTestSuite))
}

package oppybridge

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/suite"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppychain/testutil/network"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

type subscribeTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
}

func (f *subscribeTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.BondDenom = "stake"
	cfg.BondedTokens = sdk.NewInt(10000000000000000)
	cfg.StakingTokens = sdk.NewInt(100000000000000000)
	f.cfg = cfg
	f.validatorky = keyring.NewInMemory()
	// now we put the mock pool list in the test
	state := vaulttypes.GenesisState{}
	stateStaking := stakingtypes.GenesisState{}

	f.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[vaulttypes.ModuleName], &state))
	f.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateStaking))

	validators, err := genNValidator(3, f.validatorky)
	f.Require().NoError(err)
	for i := 1; i < 5; i++ {
		randPoolSk := ed25519.GenPrivKey()
		poolPubKey, err := legacybech32.MarshalPubKey(legacybech32.AccPK, randPoolSk.PubKey()) // nolint
		f.Require().NoError(err)

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
	f.Require().NoError(err)
	cfg.GenesisState[vaulttypes.ModuleName] = buf

	var stateVault stakingtypes.GenesisState
	f.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateVault))
	stateVault.Params.MaxValidators = 3
	state.Params.BlockChurnInterval = 1
	buf, err = cfg.Codec.MarshalJSON(&stateVault)
	f.Require().NoError(err)
	cfg.GenesisState[stakingtypes.ModuleName] = buf

	f.network = network.New(f.T(), cfg)

	f.Require().NotNil(f.network)

	_, err = f.network.WaitForHeight(1)
	f.Require().Nil(err)
	f.queryClient = tmservice.NewServiceClient(f.network.Validators[0].ClientCtx)
}

func (s *subscribeTestSuite) TestSubscribe() {
	oc, err := NewOppyBridge(s.network.Validators[0].APIAddress, s.network.Validators[0].RPCAddress, nil, nil)
	s.Require().NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = oc.AddSubscribe(ctx)
	s.Require().NoError(err)

	block := <-oc.CurrentNewBlockChan
	currentBlockHeight1 := block.Data.(types.EventDataNewBlock).Block.Height

	err = s.network.WaitForNextBlock()
	s.Require().NoError(err)

	block = <-oc.CurrentNewBlockChan
	currentBlockHeight2 := block.Data.(types.EventDataNewBlock).Block.Height

	s.Require().Equal(currentBlockHeight1+1, currentBlockHeight2)

	// we cache 4 blocks
	current := currentBlockHeight2 + 4
	_, err = s.network.WaitForHeight(current)
	s.Require().NoError(err)
	err = oc.RetryOppyChain()
	s.Require().NoError(err)

	current += 2
	_, err = s.network.WaitForHeight(current)
	s.Require().NoError(err)
	s.Require().Equal(4, len(oc.ChannelQueueNewBlock))
	s.Require().Equal(2, len(oc.CurrentNewBlockChan))

	// do the test again
	current += 5
	_, err = s.network.WaitForHeight(current)
	s.Require().NoError(err)
	err = oc.RetryOppyChain()
	s.Require().NoError(err)

	current += 3
	_, err = s.network.WaitForHeight(current)
	s.Require().NoError(err)

	// 11=4+5+2
	s.Require().Equal(11, len(oc.ChannelQueueNewBlock))
	s.Require().Equal(3, len(oc.CurrentNewBlockChan))
}

func (s *subscribeTestSuite) createMockChan() <-chan coretypes.ResultEvent {
	e1 := coretypes.ResultEvent{Query: "mocekquery1"}
	e2 := coretypes.ResultEvent{Query: "mocekquery2"}
	mockChan := make(chan coretypes.ResultEvent, 2)
	mockChan <- e1
	mockChan <- e2
	return mockChan
}

func (s *subscribeTestSuite) TestProcess() {
	oc, err := NewOppyBridge(s.network.Validators[0].APIAddress, s.network.Validators[0].RPCAddress, nil, nil)
	s.Require().NoError(err)

	mockChan := s.createMockChan()
	oc.CurrentNewValidator = mockChan
	oc.ProcessNewBlockChainMoreThanOne()
	s.Require().Equal(len(oc.ChannelQueueValidator), 2)
}

func TestSubscribeAndRetry(t *testing.T) {
	suite.Run(t, new(subscribeTestSuite))
}

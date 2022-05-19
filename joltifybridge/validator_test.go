package joltifybridge

import (
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain/testutil/network"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

type ValidatorTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
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

func (v *ValidatorTestSuite) SetupSuite() {
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

func (v ValidatorTestSuite) TestValidatorInitAndUpdate() {
	jc := new(JoltifyChainInstance)
	jc.grpcClient = v.network.Validators[0].ClientCtx
	err := jc.InitValidators(v.network.Validators[0].APIAddress)
	v.Require().Nil(err)

	validators, _ := jc.GetLastValidator()
	v.Require().Equal(len(validators), len(v.network.Validators))

	//sk := secp256k1.GenPrivKey()
	//remoteValidator := tmtypes.NewValidator(sk.PubKey(), 100)
	//err = jc.UpdateLatestValidator([]*tmtypes.Validator{remoteValidator}, 10)
	//v.Require().Nil(err)
	//validators, height := jc.GetLastValidator()
	//v.Require().Equal(len(v.network.Validators)+1, len(validators))
	//v.Require().Equal(int64(10), height)
	//// now we remote the last added
	//
	//remoteValidator = tmtypes.NewValidator(sk.PubKey(), 0)
	//err = jc.UpdateLatestValidator([]*vaulttypes.Validators{remoteValidator}, 10)
	//v.Require().Nil(err)
	//validators, height = jc.GetLastValidator()
	//v.Require().Equal(len(v.network.Validators), len(validators))
	//v.Require().Equal(int64(10), height)
}

func (v ValidatorTestSuite) TestQueryPool() {
	jc := new(JoltifyChainInstance)
	jc.grpcClient = v.network.Validators[0].ClientCtx
	_, err := jc.QueryLastPoolAddress()
	v.Require().NoError(err)
}

func (v ValidatorTestSuite) TestCheckWhetherSigner() {
	jc := new(JoltifyChainInstance)
	jc.grpcClient = v.network.Validators[0].ClientCtx
	jc.Keyring = v.validatorky
	blockHeight, err := GetLastBlockHeight(jc.grpcClient)
	v.Require().NoError(err)
	v.Require().GreaterOrEqual(blockHeight, int64(1))

	poolInfo, err := jc.QueryLastPoolAddress()
	v.Require().NoError(err)
	v.Require().False(len(poolInfo) == 0)
	lastPoolInfo := poolInfo[0]
	ret, err := jc.CheckWhetherSigner(lastPoolInfo)
	v.Require().NoError(err)
	v.Require().True(ret)

	err = jc.Keyring.Delete("operator")
	v.Require().NoError(err)

	_, _, err = jc.Keyring.NewMnemonic("operator", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	v.Require().NoError(err)
	ret, err = jc.CheckWhetherSigner(lastPoolInfo)
	v.Require().NoError(err)
	v.Require().False(ret)
}

func TestInitValidator(t *testing.T) {
	suite.Run(t, new(ValidatorTestSuite))
}

func (v ValidatorTestSuite) TestJoltifyChainBridge_CheckWhetherAlreadyExist() {
	jc := new(JoltifyChainInstance)
	jc.grpcClient = v.network.Validators[0].ClientCtx
	ret := jc.CheckWhetherAlreadyExist("testindex")
	v.Require().True(ret)

	ret = jc.CheckWhetherAlreadyExist("testindexnoexist")
	v.Require().False(ret)
}

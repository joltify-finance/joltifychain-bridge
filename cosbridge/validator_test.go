package cosbridge

import (
	"strconv"
	"testing"
	"time"

	cossubmit "gitlab.com/joltify/joltifychain-bridge/cos_submit"

	"gitlab.com/joltify/joltifychain-bridge/common"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	grpc1 "github.com/gogo/protobuf/grpc"
	"github.com/joltify-finance/joltify_lending/testutil/network"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

type ValidatorTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
	grpc        grpc1.ClientConn
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
	cfg.BondDenom = "stake"
	cfg.BondedTokens = sdk.NewInt(10000000000000000)
	cfg.StakingTokens = sdk.NewInt(100000000000000000)
	v.cfg = cfg
	v.validatorky = keyring.NewInMemory()
	// now we put the mock pool list in the test
	stateVaule := vaulttypes.GenesisState{}
	stateStaking := stakingtypes.GenesisState{}

	v.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[vaulttypes.ModuleName], &stateVaule))
	v.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateStaking))

	validators, err := genNValidator(3, v.validatorky)
	v.Require().NoError(err)
	height := []int{13, 15, 17, 19}
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
			PoolAddr:   randPoolSk.PubKey().Address().Bytes(),
			Nodes:      nodes,
		}
		stateVaule.CreatePoolList = append(stateVaule.CreatePoolList, &vaulttypes.CreatePool{BlockHeight: strconv.Itoa(height[i-1]), Validators: validators, Proposal: []*vaulttypes.PoolProposal{&pro}})
	}
	stateVaule.LatestTwoPool = stateVaule.CreatePoolList[:2]
	testToken := vaulttypes.IssueToken{
		Index: "testindex",
	}
	stateVaule.IssueTokenList = append(stateVaule.IssueTokenList, &testToken)

	buf, err := cfg.Codec.MarshalJSON(&stateVaule)
	v.Require().NoError(err)
	cfg.GenesisState[vaulttypes.ModuleName] = buf

	var stateVault stakingtypes.GenesisState
	v.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateVault))
	stateVault.Params.MaxValidators = 3
	stateVaule.Params.BlockChurnInterval = 2
	buf, err = cfg.Codec.MarshalJSON(&stateVault)
	v.Require().NoError(err)
	cfg.GenesisState[stakingtypes.ModuleName] = buf

	buf, err = cfg.Codec.MarshalJSON(&stateVaule)
	v.Require().NoError(err)
	cfg.GenesisState[vaulttypes.ModuleName] = buf

	v.network = network.New(v.T(), cfg)

	v.Require().NotNil(v.network)
	_, err = v.network.WaitForHeightWithTimeout(14, 5*time.Minute)
	v.Require().Nil(err)
	v.grpc = v.network.Validators[0].ClientCtx
	v.queryClient = tmservice.NewServiceClient(v.network.Validators[0].ClientCtx)
}

func (v ValidatorTestSuite) TestValidatorInitAndUpdate() {
	rp := common.NewRetryPools()
	oc, err := NewJoltifyBridge(v.network.Validators[0].APIAddress, v.network.Validators[0].RPCAddress, v.network.Validators[0].ClientCtx, nil, nil, rp)
	v.Require().NoError(err)
	err = oc.InitValidators(v.network.Validators[0].APIAddress)
	v.Require().Nil(err)

	validators, _ := oc.GetLastValidator()
	v.Require().Equal(len(validators), len(v.network.Validators))
}

func (v ValidatorTestSuite) TestQueryPool() {
	oc := new(JoltChainInstance)
	_, err := oc.QueryLastPoolAddress(v.grpc)
	v.Require().NoError(err)
}

func (v ValidatorTestSuite) TestCheckWhetherSigner() {
	oc := new(JoltChainInstance)
	a, err := cossubmit.NewCosOperations(v.network.Validators[0].APIAddress, v.network.Validators[0].RPCAddress, v.network.Validators[0].ClientCtx, v.validatorky, "joltify", nil)
	v.Require().NoError(err)
	oc.CosHandler = a
	blockHeight, err := common.GetLastBlockHeight(a.GrpcClient)
	v.Require().NoError(err)
	v.Require().GreaterOrEqual(blockHeight, int64(1))

	poolInfo, err := oc.QueryLastPoolAddress(v.grpc)
	v.Require().NoError(err)
	v.Require().False(len(poolInfo) == 0)
	lastPoolInfo := poolInfo[0]
	ret, err := oc.CheckWhetherSigner(lastPoolInfo)
	v.Require().NoError(err)
	v.Require().True(ret)

	err = oc.CosHandler.DeleteKey("operator")
	v.Require().NoError(err)

	_, err = oc.CosHandler.NewMnemonic("operator")
	v.Require().NoError(err)
	ret, err = oc.CheckWhetherSigner(lastPoolInfo)
	v.Require().NoError(err)
	v.Require().False(ret)
}

func TestInitValidator(t *testing.T) {
	suite.Run(t, new(ValidatorTestSuite))
}

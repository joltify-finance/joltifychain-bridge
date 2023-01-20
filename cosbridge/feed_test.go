package cosbridge

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/types/simulation"
	grpc1 "github.com/gogo/protobuf/grpc"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
	"gitlab.com/joltify/joltifychain-bridge/tokenlist"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/joltify-finance/joltify_lending/testutil/network"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

type FeedtransactionTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
	grpc        grpc1.ClientConn
}

func (f *FeedtransactionTestSuite) SetupSuite() {
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
	f.grpc = f.network.Validators[0].ClientCtx
	f.queryClient = tmservice.NewServiceClient(f.network.Validators[0].ClientCtx)
}

func createdTestInBoundReqs(n int) []*common.InBoundReq {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	accs := simulation.RandomAccounts(r, n)
	retReq := make([]*common.InBoundReq, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testCoin := sdk.NewCoin("test", sdk.NewInt(32))
		item := common.NewAccountInboundReq(accs[i].Address, testCoin, []byte(txid), int64(i))
		retReq[i] = &item
	}
	return retReq
}

func (f FeedtransactionTestSuite) TestFeedTransactions() {
	accs, err := generateRandomPrivKey(2)
	f.Require().NoError(err)
	tss := TssMock{
		accs[0].sk,
		// nil,
		f.network.Validators[0].ClientCtx.Keyring,
		// m.network.Validators[0].ClientCtx.Keyring,
		true,
		true,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"}, []string{"BSC"})
	f.Require().NoError(err)

	rp := common.NewRetryPools()
	oc, err := NewJoltifyBridge(f.network.Validators[0].APIAddress, f.network.Validators[0].RPCAddress, f.network.Validators[0].ClientCtx, &tss, tl, rp)
	f.Require().NoError(err)
	oc.CosHandler.Keyring = f.validatorky
	info, err := oc.CosHandler.GetKey("operator")
	f.Require().NoError(err)
	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolAddr: f.network.Validators[0].Address,
			Nodes:    []sdk.AccAddress{info.GetAddress()},
		},
	}

	acc, err := common.QueryAccount(f.grpc, f.network.Validators[0].Address.String(), "")
	f.Require().NoError(err)
	_ = acc
	pi := pubchain.Instance{
		RetryInboundReq: &sync.Map{},
		InboundReqChan:  make(chan []*common.InBoundReq, 10),
	}

	err = oc.FeedTx(f.grpc, &poolInfo, &pi)
	f.Require().NoError(err)
	f.Require().Equal(len(pi.InboundReqChan), 0)
	reqs := createdTestInBoundReqs(1)
	for _, el := range reqs {
		pi.AddInBoundItem(el)
	}

	err = oc.FeedTx(f.grpc, &poolInfo, &pi)
	f.Require().NoError(err)
	value := <-pi.InboundReqChan
	f.Require().Equal(value[0].TxID, reqs[0].TxID)

	info, err = oc.CosHandler.GetKey("operator")
	f.Require().NoError(err)
	err = oc.CosHandler.DeleteKey("operator")
	f.Require().NoError(err)

	err = oc.FeedTx(f.grpc, &poolInfo, &pi)
	f.Require().Error(err)

	_, err = oc.CosHandler.NewMnemonic("operator")
	f.Require().NoError(err)

	err = oc.FeedTx(f.grpc, &poolInfo, &pi)
	f.Require().NoError(err)
}

func TestFedTransaction(t *testing.T) {
	suite.Run(t, new(FeedtransactionTestSuite))
}

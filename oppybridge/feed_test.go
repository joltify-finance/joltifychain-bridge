package oppybridge

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/ethereum/go-ethereum/crypto"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/pubchain"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppychain/testutil/network"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

type FeedtransactionTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
}

func (f *FeedtransactionTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.BondDenom = "stake"
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
			Nodes:      nodes,
		}
		state.CreatePoolList = append(state.CreatePoolList, &vaulttypes.CreatePool{BlockHeight: strconv.Itoa(i), Validators: validators, Proposal: []*vaulttypes.PoolProposal{&pro}})
	}
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

func createdTestInBoundReqs(n int) []*common.InBoundReq {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	accs := simulation.RandomAccounts(r, n)
	retReq := make([]*common.InBoundReq, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testCoin := sdk.NewCoin("test", sdk.NewInt(32))
		sk, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		addr := crypto.PubkeyToAddress(sk.PublicKey)
		item := common.NewAccountInboundReq(accs[i].Address, addr, testCoin, []byte(txid), int64(i), int64(i))
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
	tl, err := createMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	f.Require().NoError(err)
	oc, err := NewOppyBridge(f.network.Validators[0].APIAddress, f.network.Validators[0].RPCAddress, &tss, tl)
	f.Require().NoError(err)
	oc.Keyring = f.validatorky
	oc.grpcClient = f.network.Validators[0].ClientCtx
	info, err := oc.Keyring.Key("operator")
	f.Require().NoError(err)
	poolInfo := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolAddr: f.network.Validators[0].Address,
			Nodes:    []sdk.AccAddress{info.GetAddress()},
		},
	}

	acc, err := queryAccount(f.network.Validators[0].Address.String(), oc.grpcClient)
	f.Require().NoError(err)
	_ = acc
	pi := pubchain.Instance{
		RetryInboundReq: &sync.Map{},
		InboundReqChan:  make(chan *common.InBoundReq, 10),
	}

	err = oc.FeedTx(&poolInfo, &pi, 100)
	f.Require().NoError(err)
	f.Require().Equal(len(pi.InboundReqChan), 0)
	reqs := createdTestInBoundReqs(1)
	for _, el := range reqs {
		pi.AddItem(el)
	}

	err = oc.FeedTx(&poolInfo, &pi, 100)
	f.Require().NoError(err)
	value := <-pi.InboundReqChan
	f.Require().Equal(value.TxID, reqs[0].TxID)
}

func TestFedTransaction(t *testing.T) {
	suite.Run(t, new(FeedtransactionTestSuite))
}

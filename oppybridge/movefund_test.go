package oppybridge

import (
	"context"
	"strconv"
	"testing"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
	"github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppychain/testutil/network"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

type MoveFundTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
}

func (m *MoveFundTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.BondDenom = "stake"
	cfg.MinGasPrices = "0stake"
	config.ChainID = cfg.ChainID
	m.cfg = cfg
	m.validatorky = keyring.NewInMemory()
	// now we put the mock pool list in the test
	state := vaulttypes.GenesisState{}
	stateStaking := stakingtypes.GenesisState{}

	m.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[vaulttypes.ModuleName], &state))
	m.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateStaking))

	validators, err := genNValidator(3, m.validatorky)
	m.Require().NoError(err)
	for i := 1; i < 5; i++ {
		randPoolSk := ed25519.GenPrivKey()
		poolPubKey, err := legacybech32.MarshalPubKey(legacybech32.AccPK, randPoolSk.PubKey()) // nolint
		m.Require().NoError(err)

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
	m.Require().NoError(err)
	cfg.GenesisState[vaulttypes.ModuleName] = buf

	var stateVault stakingtypes.GenesisState
	m.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateVault))
	stateVault.Params.MaxValidators = 3
	state.Params.BlockChurnInterval = 1
	buf, err = cfg.Codec.MarshalJSON(&stateVault)
	m.Require().NoError(err)
	cfg.GenesisState[stakingtypes.ModuleName] = buf

	m.network = network.New(m.T(), cfg)

	m.Require().NotNil(m.network)

	_, err = m.network.WaitForHeight(1)
	m.Require().Nil(err)

	m.queryClient = tmservice.NewServiceClient(m.network.Validators[0].ClientCtx)
}

func TestMoveFund(t *testing.T) {
	suite.Run(t, new(MoveFundTestSuite))
}

func (m MoveFundTestSuite) TestMoveFunds() {
	accs, err := generateRandomPrivKey(4)
	m.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		m.network.Validators[0].ClientCtx.Keyring,
		true,
		true,
	}
	tl, err := createMockTokenlist("testAddr", "testDenom")
	m.Require().NoError(err)
	jc, err := NewOppyBridge(m.network.Validators[0].RPCAddress, m.network.Validators[0].RPCAddress, &tss, tl)
	m.Require().NoError(err)
	defer func() {
		err2 := jc.TerminateBridge()
		if err2 != nil {
			jc.logger.Error().Err(err2).Msgf("fail to terminate the bridge")
		}
	}()
	err = m.network.WaitForNextBlock()
	m.Require().NoError(err)

	// we need to add this as it seems the rpcaddress is incorrect
	jc.grpcClient = m.network.Validators[0].ClientCtx
	jc.Keyring = m.validatorky

	info, _ := m.network.Validators[0].ClientCtx.Keyring.Key("node0")
	pk := info.GetPubKey()
	pkstr := legacybech32.MustMarshalPubKey(legacybech32.AccPK, pk) // nolint
	valAddr, err := misc.PoolPubKeyToJoltAddress(pkstr)
	m.Require().NoError(err)

	pro := vaulttypes.PoolProposal{
		Nodes: []sdk.AccAddress{info.GetAddress()},
	}

	poolinfo := common.PoolInfo{
		OppyAddress: valAddr,
		Pk:          pkstr,
		PoolInfo:    &vaulttypes.PoolInfo{CreatePool: &pro},
	}
	acc, err := queryAccount(info.GetAddress().String(), jc.grpcClient)
	m.Require().NoError(err)

	coin := sdk.NewCoin("stake", sdk.NewInt(100))
	msg := types.NewMsgSend(valAddr, accs[0].joltAddr, sdk.NewCoins(coin))
	txBuilder, err := Gensigntx(jc, []sdk.Msg{msg}, info, acc.GetAccountNumber(), acc.GetSequence(), m.network.Validators[0].ClientCtx.Keyring)
	m.Require().NoError(err)
	txBytes, err := jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	m.Require().NoError(err)
	ret, _, err := jc.BroadcastTx(context.Background(), txBytes, false)
	m.Require().NoError(err)
	m.Require().True(ret)

	jc.AddMoveFundItem(&poolinfo, 20)
	ret = jc.MoveFound(20, accs[0].joltAddr)
	m.Require().False(ret)

	// we are not the sisnger
	ret = jc.MoveFound(300, accs[0].joltAddr)
	m.Require().False(ret)

	info2, err := m.validatorky.Key("operator")
	m.Require().NoError(err)
	pro2 := vaulttypes.PoolProposal{
		Nodes: []sdk.AccAddress{info2.GetAddress()},
	}

	poolinfo.PoolInfo = &vaulttypes.PoolInfo{CreatePool: &pro2}

	jc.AddMoveFundItem(&poolinfo, 21)
	// we are not the sisnger
	ret = jc.MoveFound(300, accs[0].joltAddr)
	m.Require().True(ret)
}

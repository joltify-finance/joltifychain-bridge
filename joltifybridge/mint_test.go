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
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
	"gitlab.com/joltify/joltifychain/testutil/network"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

type MintTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
}

func (v *MintTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.MinGasPrices = "0stake"
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
	v.cfg = cfg

	v.Require().NotNil(v.network)

	_, err = v.network.WaitForHeight(1)
	v.Require().Nil(err)

	v.queryClient = tmservice.NewServiceClient(v.network.Validators[0].ClientCtx)
}

func (m MintTestSuite) TestPrepareIssueTokenRequest() {
	accs, err := generateRandomPrivKey(3)
	m.Require().NoError(err)
	tx := pubchain.NewAccountInboundReq(accs[0].joltAddr, accs[1].commAddr, sdk.NewCoin("test", sdk.NewInt(1)), []byte("test"), int64(100))
	_, err = prepareIssueTokenRequest(&tx, accs[2].commAddr.String(), "1")
	m.Require().EqualError(err, "decoding bech32 failed: string not all lowercase or all uppercase")

	_, err = prepareIssueTokenRequest(&tx, accs[2].joltAddr.String(), "1")

	m.Require().NoError(err)
}

func (m MintTestSuite) TestProcessInbound() {
	accs, err := generateRandomPrivKey(2)
	m.Require().NoError(err)
	tss := TssMock{
		nil,
		m.network.Validators[0].ClientCtx.Keyring,
		true,
		true,
	}
	jc, err := NewJoltifyBridge(m.network.Validators[0].APIAddress, m.network.Validators[0].RPCAddress, &tss)
	m.Require().NoError(err)
	jc.Keyring = m.validatorky

	// we need to add this as it seems the rpcaddress is incorrect
	jc.grpcClient = m.network.Validators[0].ClientCtx
	defer func() {
		err := jc.TerminateBridge()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()

	_, err = m.network.WaitForHeightWithTimeout(10, time.Second*30)
	m.Require().NoError(err)

	info, _ := m.network.Validators[0].ClientCtx.Keyring.Key("node0")
	// pk := m.network.Validators[0].PubKey
	pk := info.GetPubKey()
	pkstr := legacybech32.MustMarshalPubKey(legacybech32.AccPK, pk) // nolint
	valAddr, err := misc.PoolPubKeyToJoltAddress(pkstr)
	m.Require().NoError(err)
	tx := pubchain.NewAccountInboundReq(m.network.Validators[0].Address, accs[0].commAddr, sdk.NewCoin("test", sdk.NewInt(1)), []byte("test"), int64(100))
	_, _, err = jc.ProcessInBound(&tx)
	m.Require().EqualError(err, "not enough signer")

	// need to be called twice

	// err = jc.CreatePoolAccInfo(m.network.Validators[0].Address.String())
	// m.Require().NoError(err)
	send := banktypes.NewMsgSend(valAddr, accs[0].joltAddr, sdk.Coins{sdk.NewCoin("stake", sdk.NewInt(1))})

	acc, err := queryAccount(accs[0].joltAddr.String(), jc.grpcClient)
	m.Require().NoError(err)
	txBuilder, err := jc.genSendTx([]sdk.Msg{send}, acc.GetSequence(), acc.GetAccountNumber(), 200000, nil)
	m.Require().NoError(err)
	txBytes, err := jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	m.Require().NoError(err)
	ret, _, err := jc.BroadcastTx(context.Background(), txBytes)
	m.Require().NoError(err)
	m.Require().True(ret)

	pool := common.PoolInfo{
		Pk:             accs[0].pk,
		JoltifyAddress: accs[0].joltAddr,
		EthAddress:     accs[0].commAddr,
	}

	jc.lastTwoPools[0] = &pool
	jc.lastTwoPools[1] = &pool

	// err = jc.CreatePoolAccInfo(accs[0].joltAddr.String())
	// m.Require().NoError(err)
	_, _, err = jc.ProcessInBound(&tx)
	m.Require().Error(err)
}

func TestMint(t *testing.T) {
	suite.Run(t, new(MintTestSuite))
}

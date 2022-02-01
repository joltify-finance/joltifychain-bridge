package joltifybridge

import (
	"context"
	"encoding/base64"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/joltgeorge/tss/common"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"gitlab.com/joltify/joltifychain/testutil/network"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
	"golang.org/x/crypto/sha3"
)

type BridgeTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
}

func (b *BridgeTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.MinGasPrices = "0stake"
	b.validatorky = keyring.NewInMemory()

	// now we put the mock pool list in the test
	state := vaulttypes.GenesisState{}
	stateStaking := stakingtypes.GenesisState{}
	b.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[vaulttypes.ModuleName], &state))
	b.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateStaking))

	validators, err := genNValidator(3, b.validatorky)
	b.Require().NoError(err)
	for i := 1; i < 50; i++ {
		randPoolSk := ed25519.GenPrivKey()
		poolPubKey, err := legacybech32.MarshalPubKey(legacybech32.AccPK, randPoolSk.PubKey()) // nolint
		b.Require().NoError(err)

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
	b.Require().NoError(err)
	cfg.GenesisState[vaulttypes.ModuleName] = buf

	var stateVault stakingtypes.GenesisState
	b.Require().NoError(cfg.Codec.UnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stateVault))
	stateVault.Params.MaxValidators = 3
	state.Params.BlockChurnInterval = 2
	buf, err = cfg.Codec.MarshalJSON(&stateVault)
	b.Require().NoError(err)
	cfg.GenesisState[stakingtypes.ModuleName] = buf

	b.network = network.New(b.T(), cfg)
	b.cfg = cfg

	b.Require().NotNil(b.network)

	_, err = b.network.WaitForHeight(1)
	b.Require().Nil(err)
	b.queryClient = tmservice.NewServiceClient(b.network.Validators[0].ClientCtx)
}

func (b BridgeTestSuite) TestBridge() {
	tss := TssMock{
		nil,
		b.network.Validators[0].ClientCtx.Keyring,
		true,
		false,
	}
	jc, err := NewJoltifyBridge(b.network.Validators[0].APIAddress, b.network.Validators[0].RPCAddress, &tss)
	b.Require().NoError(err)
	jc.Keyring = b.validatorky

	// we need to add this as it seems the rpcaddress is incorrect
	jc.grpcClient = b.network.Validators[0].ClientCtx
	defer func() {
		err := jc.TerminateBridge()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()

	tssNodeid := jc.GetTssNodeID()
	b.Require().Greater(len(tssNodeid), 1)

	h := sha3.New256()
	h.Write([]byte("123"))
	msg := base64.StdEncoding.EncodeToString(h.Sum(nil))
	tssMsg := tssclient.TssSignigMsg{Pk: "test", Msgs: []string{msg}, Signers: []string{"1", "2"}, BlockHeight: int64(2), Version: "0.15.6"}
	_, err = jc.doTssSign(&tssMsg)
	b.Require().Error(err)
	_, err = b.network.WaitForHeightWithTimeout(30, time.Minute)
	b.Require().NoError(err)
	tss.keysignSuccess = true
	ret, _ := jc.doTssSign(&tssMsg)
	b.Require().Equal(ret.Status, common.Success)
}

func (b BridgeTestSuite) TestBridgeTx() {
	accs, err := generateRandomPrivKey(3)
	b.Require().NoError(err)
	tss := TssMock{
		accs[0].sk,
		b.network.Validators[0].ClientCtx.Keyring,
		true,
		true,
	}
	jc, err := NewJoltifyBridge(b.network.Validators[0].APIAddress, b.network.Validators[0].RPCAddress, &tss)
	b.Require().NoError(err)
	jc.Keyring = b.validatorky

	// we need to add this as it seems the rpcaddress is incorrect
	jc.grpcClient = b.network.Validators[0].ClientCtx
	defer func() {
		err := jc.TerminateBridge()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()

	tmsg := vaulttypes.MsgCreateCreatePool{
		Creator:     b.network.Validators[0].Address,
		PoolPubKey:  accs[1].pk,
		BlockHeight: "5",
	}
	err = jc.CreatePoolAccInfo(b.network.Validators[0].Address.String())
	b.Require().NoError(err)
	num, seq := jc.AcquirePoolAccountInfo()

	_, err = b.network.WaitForHeightWithTimeout(5, time.Minute*5)
	b.Require().NoError(err)
	gas, err := jc.GasEstimation([]sdk.Msg{&tmsg}, seq, nil)
	b.Require().NoError(err)
	b.Require().Greater(gas, uint64(0))
	_, err = jc.genSendTx([]sdk.Msg{&tmsg}, seq, num, gas, nil)
	b.Require().NoError(err)

	h := sha3.New256()
	h.Write([]byte("123"))
	msg := base64.StdEncoding.EncodeToString(h.Sum(nil))
	info, err := jc.Keyring.Key("operator")
	b.Require().NoError(err)
	legacybech32.UnmarshalPubKey(legacybech32.AccPK, info.GetPubKey().String()) // nolint
	mpk := secp256k1.PubKey{
		Key: info.GetPubKey().Bytes(),
	}
	pk, err := legacybech32.MarshalPubKey(legacybech32.AccPK, &mpk) // nolint
	b.Require().NoError(err)
	tssMsg := tssclient.TssSignigMsg{Pk: pk, Msgs: []string{msg}, Signers: []string{"1", "2"}, BlockHeight: int64(2), Version: "0.15.6"}
	txBuilder, err := jc.genSendTx([]sdk.Msg{&tmsg}, seq, num, gas, &tssMsg)
	b.Require().NoError(err)

	txBytes, err := jc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	b.Require().NoError(err)
	_, _, err = jc.BroadcastTx(context.Background(), txBytes)
	b.Require().NoError(err)
	_, err = b.network.WaitForHeightWithTimeout(10, time.Second*30)
	b.Require().NoError(err)
	bh, err := jc.GetLastBlockHeight()
	b.Require().NoError(err)
	b.Require().Greater(bh, int64(0))
	err = jc.prepareTssPool(b.network.Validators[0].Address, accs[1].pk, "10")
	b.Require().NoError(err)
}

func (b BridgeTestSuite) TestCheckAndUpdatePool() {
	accs, err := generateRandomPrivKey(3)
	b.Require().NoError(err)

	keyInfo, err := b.network.Validators[0].ClientCtx.Keyring.Key("node0")
	b.Require().NoError(err)

	tss := TssMock{
		accs[0].sk,
		b.network.Validators[0].ClientCtx.Keyring,
		true,
		true,
	}
	jc, err := NewJoltifyBridge(b.network.Validators[0].APIAddress, b.network.Validators[0].RPCAddress, &tss)
	b.Require().NoError(err)
	jc.Keyring = b.validatorky

	// we need to add this as it seems the rpcaddress is incorrect
	jc.grpcClient = b.network.Validators[0].ClientCtx
	defer func() {
		err := jc.TerminateBridge()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()

	creatorPk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, keyInfo.GetPubKey()) // nolint
	_, err = b.network.WaitForHeightWithTimeout(10, time.Second*30)
	b.Require().NoError(err)
	err = jc.prepareTssPool(b.network.Validators[0].Address, creatorPk, "10")
	b.Require().NoError(err)
	b.Require().Equal(len(jc.msgSendCache), 1)
	ret, _ := jc.CheckAndUpdatePool(10)
	b.Require().False(ret)
}

func TestBridge(t *testing.T) {
	suite.Run(t, new(BridgeTestSuite))
}

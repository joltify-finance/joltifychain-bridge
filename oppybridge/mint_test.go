package oppybridge

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	grpc1 "github.com/gogo/protobuf/grpc"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppychain/testutil/network"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

type MintTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
	grpc        grpc1.ClientConn
}

func (v *MintTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	cfg := network.DefaultConfig()
	cfg.BondDenom = "stake"
	cfg.BondedTokens = sdk.NewInt(10000000000000000)
	cfg.StakingTokens = sdk.NewInt(100000000000000000)
	config.ChainID = cfg.ChainID
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
	v.grpc = v.network.Validators[0].ClientCtx
	v.queryClient = tmservice.NewServiceClient(v.network.Validators[0].ClientCtx)
}

func (m MintTestSuite) TestPrepareIssueTokenRequest() {
	accs, err := generateRandomPrivKey(3)
	m.Require().NoError(err)
	tx := common.NewAccountInboundReq(accs[0].oppyAddr, accs[1].commAddr, sdk.NewCoin("test", sdk.NewInt(1)), []byte("test"), int64(2))
	_, err = prepareIssueTokenRequest(&tx, accs[2].commAddr.String(), "1")
	m.Require().EqualError(err, "decoding bech32 failed: string not all lowercase or all uppercase")

	_, err = prepareIssueTokenRequest(&tx, accs[2].oppyAddr.String(), "1")

	m.Require().NoError(err)
}

func Gensigntx(oc *OppyChainInstance, sdkMsg []sdk.Msg, key keyring.Info, accNum, accSeq uint64, signkeyring keyring.Keyring, memo string) (client.TxBuilder, error) {
	encCfg := *oc.encoding
	// Create a new TxBuilder.
	txBuilder := encCfg.TxConfig.NewTxBuilder()

	err := txBuilder.SetMsgs(sdkMsg...)
	if err != nil {
		return nil, err
	}

	// we use the default here
	txBuilder.SetGasLimit(500000)
	txBuilder.SetMemo(memo)

	var sigV2 signing.SignatureV2

	sigV2 = signing.SignatureV2{
		PubKey: key.GetPubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  encCfg.TxConfig.SignModeHandler().DefaultMode(),
			Signature: nil,
		},
		Sequence: accSeq,
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, err
	}

	signerData := xauthsigning.SignerData{
		ChainID:       config.ChainID,
		AccountNumber: accNum,
		Sequence:      accSeq,
	}

	signMode := encCfg.TxConfig.SignModeHandler().DefaultMode()
	// Generate the bytes to be signed.
	signBytes, err := encCfg.TxConfig.SignModeHandler().GetSignBytes(signMode, signerData, txBuilder.GetTx())
	if err != nil {
		return nil, err
	}

	signature, pk, err := signkeyring.Sign("node0", signBytes)
	if err != nil {
		return nil, err
	}
	sigData := signing.SingleSignatureData{
		SignMode:  signMode,
		Signature: signature,
	}

	sigV2 = signing.SignatureV2{
		PubKey:   pk,
		Data:     &sigData,
		Sequence: signerData.Sequence,
	}
	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		oc.logger.Error().Err(err).Msgf("fail to set the signature")
		return nil, err
	}

	return txBuilder, nil
}

//nolint:funlen
func (m MintTestSuite) TestProcessInbound() {
	accs, err := generateRandomPrivKey(2)
	m.Require().NoError(err)
	tss := TssMock{
		accs[0].sk,
		// nil,
		m.network.Validators[0].ClientCtx.Keyring,
		// m.network.Validators[0].ClientCtx.Keyring,
		true,
		true,
	}
	tl, err := tokenlist.CreateMockTokenlist([]string{"testAddr"}, []string{"testDenom"})
	m.Require().NoError(err)
	oc, err := NewOppyBridge(m.network.Validators[0].APIAddress, m.network.Validators[0].RPCAddress, &tss, tl)
	m.Require().NoError(err)
	oc.Keyring = m.validatorky

	// we need to add this as it seems the rpcaddress is incorrect
	oc.GrpcClient = m.network.Validators[0].ClientCtx
	defer func() {
		err := oc.TerminateBridge()
		if err != nil {
			oc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()

	_, err = m.network.WaitForHeightWithTimeout(10, time.Second*30)
	m.Require().NoError(err)

	// err = oc.InitValidators(m.network.Validators[0].APIAddress)
	// m.Require().NoError(err)

	info, _ := m.network.Validators[0].ClientCtx.Keyring.Key("node0")
	pk := info.GetPubKey()
	pkstr := legacybech32.MustMarshalPubKey(legacybech32.AccPK, pk) // nolint
	valAddr, err := misc.PoolPubKeyToOppyAddress(pkstr)
	m.Require().NoError(err)

	acc, err := queryAccount(m.grpc, valAddr.String(), m.network.Validators[0].RPCAddress)
	m.Require().NoError(err)
	tx := common.NewAccountInboundReq(valAddr, accs[0].commAddr, sdk.NewCoin("test", sdk.NewInt(1)), []byte("test"), int64(100))

	tx.SetAccountInfo(0, 0, accs[0].oppyAddr, accs[0].pk)

	send := banktypes.NewMsgSend(valAddr, accs[0].oppyAddr, sdk.Coins{sdk.NewCoin("stake", sdk.NewInt(1))})

	txBuilder, err := Gensigntx(oc, []sdk.Msg{send}, info, acc.GetAccountNumber(), acc.GetSequence(), m.network.Validators[0].ClientCtx.Keyring, "")
	m.Require().NoError(err)
	txBytes, err := oc.encoding.TxConfig.TxEncoder()(txBuilder.GetTx())
	m.Require().NoError(err)
	ret, _, err := oc.BroadcastTx(context.Background(), m.grpc, txBytes, false)
	m.Require().NoError(err)
	m.Require().True(ret)

	pool := common.PoolInfo{
		Pk:          accs[0].pk,
		OppyAddress: accs[0].oppyAddr,
		EthAddress:  accs[0].commAddr,
	}

	oc.lastTwoPools[0] = &pool
	oc.lastTwoPools[1] = &pool

	_, err = oc.DoProcessInBound(m.grpc, []*common.InBoundReq{&tx})
	m.Require().NoError(err)
}

func TestMint(t *testing.T) {
	suite.Run(t, new(MintTestSuite))
}

package joltifybridge

import (
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" // nolint
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain/testutil/network"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

type OutBoundTestSuite struct {
	suite.Suite
	cfg         network.Config
	network     *network.Network
	validatorky keyring.Keyring
	queryClient tmservice.ServiceClient
}

func (v *OutBoundTestSuite) SetupSuite() {
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

type account struct {
	sk       *secp256k1.PrivKey
	pk       string
	joltAddr sdk.AccAddress
	commAddr common.Address
}

func generateRandomPrivKey(n int) ([]account, error) {
	randomAccounts := make([]account, n)
	for i := 0; i < n; i++ {
		sk := secp256k1.GenPrivKey()
		pk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, sk.PubKey()) // nolint

		ethAddr, err := misc.PoolPubKeyToEthAddress(pk)
		if err != nil {
			return nil, err
		}
		addrJolt, err := sdk.AccAddressFromHex(sk.PubKey().Address().String())
		if err != nil {
			return nil, err
		}
		tAccount := account{
			sk,
			pk,
			addrJolt,
			ethAddr,
		}
		randomAccounts[i] = tAccount
	}
	return randomAccounts, nil
}

func (o OutBoundTestSuite) TestUpdatePool() {
	var err error
	accs, err := generateRandomPrivKey(2)
	o.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}
	//
	jc, err := NewJoltifyBridge(o.network.Validators[0].APIAddress, o.network.Validators[0].RPCAddress, &tss)
	o.Require().NoError(err)
	defer func() {
		err := jc.TerminateBridge()
		if err != nil {
			jc.logger.Error().Err(err).Msgf("fail to terminate the bridge")
		}
	}()
	//
	key, _, err := jc.Keyring.NewMnemonic("pooltester1", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	o.Require().NoError(err)

	key2, _, err := jc.Keyring.NewMnemonic("pooltester2", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	o.Require().NoError(err)

	key3, _, err := jc.Keyring.NewMnemonic("pooltester3", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	o.Require().NoError(err)

	cospk := legacybech32.MustMarshalPubKey(legacybech32.AccPK, key.GetPubKey()) // nolint
	jc.UpdatePool(cospk)
	pubkeyStr := key.GetPubKey().Address().String()
	pk, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	pools := jc.GetPool()
	o.Require().Nil(pools[0])
	addr2 := pools[1].JoltifyAddress
	o.Require().True(pk.Equals(addr2))
	// now we add another pool
	cospk = legacybech32.MustMarshalPubKey(legacybech32.AccPK, key2.GetPubKey()) // nolint
	jc.UpdatePool(cospk)
	pools = jc.GetPool()
	pubkeyStr = key2.GetPubKey().Address().String()
	pk2, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	o.Require().True(pk.Equals(pools[0].JoltifyAddress))

	pool1 := jc.lastTwoPools[1].JoltifyAddress
	o.Require().True(pk2.Equals(pool1))

	// now we add another pool and pop out the firt one

	cospk = legacybech32.MustMarshalPubKey(legacybech32.AccPK, key3.GetPubKey()) // nolint
	jc.UpdatePool(cospk)
	pools = jc.GetPool()

	pubkeyStr = key3.GetPubKey().Address().String()
	pk3, err := sdk.AccAddressFromHex(pubkeyStr)
	o.Require().NoError(err)
	o.Require().True(pk3.Equals(pools[1].JoltifyAddress))

	time.Sleep(time.Second)
}

func (o OutBoundTestSuite) TestOutBoundReq() {
	accs, err := generateRandomPrivKey(2)
	o.Require().NoError(err)
	boundReq := newOutboundReq(accs[0].commAddr, accs[1].commAddr, sdk.NewCoin("testcoing", sdk.NewInt(1)), 10)
	boundReq.SetItemHeight(100)
	a, b, _, h := boundReq.GetOutBoundInfo()
	o.Require().Equal(a.String(), accs[0].commAddr.String())
	o.Require().Equal(b.String(), accs[1].commAddr.String())
	o.Require().Equal(h, int64(100))
}

func (o OutBoundTestSuite) TestOutTx() {
	accs, err := generateRandomPrivKey(2)
	o.Require().NoError(err)
	tx := pubchain.outboundTx{
		accs[0].commAddr,
		100,
		sdk.NewCoin("test", sdk.NewInt(1)),
		sdk.NewCoin("fee", sdk.NewInt(10)),
	}
	err = tx.Verify()
	o.Require().Errorf(err, "invalid outbound fee denom")
	tx.fee = sdk.NewCoin(config.OutBoundDenomFee, sdk.NewInt(1))
	err = tx.Verify()
	o.Require().Error(err, "the fee is not enough with 1<10")

	tx.fee = sdk.NewCoin(config.OutBoundDenomFee, sdk.NewInt(100))
	err = tx.Verify()
	o.Require().NoError(err)
}

func (o OutBoundTestSuite) TestProcessMsg() {
	accs, err := generateRandomPrivKey(4)
	o.Assert().NoError(err)
	tss := TssMock{
		accs[0].sk,
		nil,
		true,
		true,
	}

	jc, err := NewJoltifyBridge(o.network.Validators[0].RPCAddress, o.network.Validators[0].RPCAddress, &tss)
	o.Require().NoError(err)
	defer func() {
		err2 := jc.TerminateBridge()
		if err2 != nil {
			jc.logger.Error().Err(err2).Msgf("fail to terminate the bridge")
		}
	}()

	// we need to add this as it seems the rpcaddress is incorrect
	jc.grpcClient = o.network.Validators[0].ClientCtx
	baseBlockHeight := int64(100)
	msg := banktypes.MsgSend{}

	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().EqualError(err, "empty address string is not allowed")

	msg.FromAddress = o.network.Validators[0].Address.String()
	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().EqualError(err, "empty address string is not allowed")

	ret := jc.CheckWhetherAlreadyExist("testindex")
	o.Require().True(ret)

	msg.ToAddress = accs[3].joltAddr.String()
	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().EqualError(err, "not a top up message to the pool")

	msg.ToAddress = accs[1].joltAddr.String()
	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().EqualError(err, "we only allow fee and top up in one tx now")

	coin1 := sdk.NewCoin(config.OutBoundDenom, sdk.NewInt(100))
	coin2 := sdk.NewCoin(config.OutBoundDenomFee, sdk.NewInt(1))
	coin3 := sdk.NewCoin(config.InBoundDenomFee, sdk.NewInt(100))
	coin4 := sdk.NewCoin(config.OutBoundDenomFee, sdk.NewInt(100))

	msg.Amount = sdk.NewCoins(coin1, coin3)
	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().EqualError(err, "invalid fee pair")
	msg.Amount = sdk.NewCoins(coin2, coin3)
	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().EqualError(err, "invalid fee pair")

	msg.Amount = sdk.NewCoins(coin1, coin2)
	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().EqualError(err, "not enough fee")

	msg.Amount = sdk.NewCoins(coin1, coin4)
	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().NoError(err)

	// we set the wrong account
	msg.FromAddress = accs[1].commAddr.String()
	err = jc.processMsg(baseBlockHeight, []sdk.AccAddress{accs[1].joltAddr, accs[2].joltAddr}, accs[3].commAddr, &msg, []byte("msg1"))
	o.Require().EqualError(err, "rpc error: code = InvalidArgument desc = decoding bech32 failed: string not all lowercase or all uppercase: invalid request")
}

func TestTxOutBound(t *testing.T) {
	suite.Run(t, new(OutBoundTestSuite))
}

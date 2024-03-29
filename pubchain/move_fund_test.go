package pubchain

import (
	"context"
	"encoding/hex"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
	"github.com/stretchr/testify/suite"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/tokenlist"
)

type TestNetTestSuite struct {
	suite.Suite
	pubChain *Instance
	pk1      string
	sk1      *secp256k1.PrivKey
	pk2      string
	sk2      *secp256k1.PrivKey
	tss      *TssMock
}

func (tn *TestNetTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
	// tokenAddrTest := "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"

	a1 := "c225ac7cf268405670c004e0b8f6b7df5fefb80f3505aaf9619ea89c787a67e7"
	a2 := "481d305c7be328b6defd500209f9fdfb5447231f4c1f665324df951029506e12"
	data, err := hex.DecodeString(a1)
	if err != nil {
		panic(err)
	}
	sk := secp256k1.PrivKey{Key: data}
	pk, err := legacybech32.MarshalPubKey(legacybech32.AccPK, sk.PubKey()) //nolint
	if err != nil {
		panic(err)
	}
	tn.pk1 = pk
	tn.sk1 = &sk

	data, err = hex.DecodeString(a2)
	if err != nil {
		panic(err)
	}
	sk2 := secp256k1.PrivKey{Key: data}
	pk2, err := legacybech32.MarshalPubKey(legacybech32.AccPK, sk2.PubKey()) //nolint
	if err != nil {
		panic(err)
	}

	tn.pk2 = pk2
	tn.sk2 = &sk2

	tss := TssMock{sk: &sk}
	tn.tss = &tss
	tl, err := tokenlist.CreateMockTokenlist([]string{"0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"}, []string{"JUSD"}, []string{"BSC"})
	if err != nil {
		panic(err)
	}
	wg := sync.WaitGroup{}
	mockRetry := sync.Map{}
	pubChain, err := NewChainInstance(misc.WebsocketTest, misc.WebsocketTest, &tss, tl, &wg, &mockRetry)
	if err != nil {
		panic(err)
	}

	tn.pubChain = pubChain
}

func (tn TestNetTestSuite) TestProcessNewBlock() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	tn.pubChain.BSCChain.ChainLocker.Lock()
	number, err := tn.pubChain.BSCChain.Client.BlockNumber(ctx)
	tn.Require().NoError(err)
	tn.pubChain.BSCChain.ChainLocker.Unlock()
	err = tn.pubChain.ProcessNewBlock("BSC", tn.pubChain.BSCChain, big.NewInt(int64(number)), tn.pubChain.FeeModule, "")
	tn.Require().NoError(err)
}

func (tn TestNetTestSuite) TestDoMoveFund() {
	addr1, err := misc.PoolPubKeyToEthAddress(tn.pk1)
	tn.Require().NoError(err)
	addr2, err := misc.PoolPubKeyToEthAddress(tn.pk2)
	tn.Require().NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	balance1, err := tn.pubChain.BSCChain.getBalanceWithLock(ctx, addr1)
	tn.Require().NoError(err)
	balance2, err := tn.pubChain.BSCChain.getBalanceWithLock(ctx, addr2)
	tn.Require().NoError(err)

	tn.tss.sk = tn.sk2
	previouspool := tn.pk2
	newpool := tn.pk1

	if balance1.Cmp(balance2) == 1 {
		tn.tss.sk = tn.sk1
		previouspool = tn.pk1
		newpool = tn.pk2
	}

	ethAddr, err := misc.PoolPubKeyToEthAddress(previouspool)
	tn.Require().NoError(err)
	oppyAddr, err := misc.PoolPubKeyToOppyAddress(previouspool)
	tn.Require().NoError(err)

	previous := bcommon.PoolInfo{
		Pk:         previouspool,
		CosAddress: oppyAddr,
		EthAddress: ethAddr,
	}

	poolInfo2 := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: newpool,
			Nodes:      nil,
		},
	}

	err = tn.pubChain.UpdatePool(&poolInfo2)
	tn.Require().NoError(err)
	err = tn.pubChain.UpdatePool(&poolInfo2)
	tn.Require().NoError(err)

	ret := tn.pubChain.MoveFound(100, tn.pubChain.BSCChain, &previous, tn.pubChain.BSCChain.Client)

	if !ret {
		item, height := tn.pubChain.PopMoveFundItemAfterBlock(101, "BSC")
		tn.Require().Equal(int64(0), height)
		tn.Require().Nil(item)
		item, _ = tn.pubChain.PopMoveFundItemAfterBlock(100+config.MINCHECKBLOCKGAP+100, "BSC")
		tn.Require().NotNil(item)
		ret2 := tn.pubChain.MoveFound(100, tn.pubChain.BSCChain, item.PoolInfo, tn.pubChain.BSCChain.Client)
		tn.Require().True(ret2)
	}
}

func TestEvent(t *testing.T) {
	suite.Run(t, new(TestNetTestSuite))
}

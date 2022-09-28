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
	"github.com/stretchr/testify/suite"
	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/config"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
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
	tl, err := tokenlist.CreateMockTokenlist([]string{"0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"}, []string{"JUSD"})
	if err != nil {
		panic(err)
	}
	wg := sync.WaitGroup{}
	pubChain, err := NewChainInstance(misc.WebsocketTest, &tss, tl, &wg)
	if err != nil {
		panic(err)
	}

	tn.pubChain = pubChain
}

func (tn TestNetTestSuite) TestProcessNewBlock() {
	err := tn.pubChain.ProcessNewBlock(big.NewInt(22998501))
	tn.Require().NoError(err)
}

func (tn TestNetTestSuite) TestDoMoveFund() {
	addr1, err := misc.PoolPubKeyToEthAddress(tn.pk1)
	tn.Require().NoError(err)
	addr2, err := misc.PoolPubKeyToEthAddress(tn.pk2)
	tn.Require().NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	balance1, err := tn.pubChain.getBalanceWithLock(ctx, addr1)
	tn.Require().NoError(err)
	balance2, err := tn.pubChain.getBalanceWithLock(ctx, addr2)
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
		Pk:          previouspool,
		OppyAddress: oppyAddr,
		EthAddress:  ethAddr,
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

	ret := tn.pubChain.MoveFound(100, misc.WebsocketTest, &previous, tn.pubChain.EthClient)

	if !ret {
		item, height := tn.pubChain.PopMoveFundItemAfterBlock(101)
		tn.Require().Equal(int64(0), height)
		tn.Require().Nil(item)
		item, _ = tn.pubChain.PopMoveFundItemAfterBlock(100 + config.MINCHECKBLOCKGAP + 100)
		tn.Require().NotNil(item)
		ret2 := tn.pubChain.MoveFound(100, misc.WebsocketTest, item, tn.pubChain.EthClient)
		tn.Require().True(ret2)
	}
}

func TestEvent(t *testing.T) {
	suite.Run(t, new(TestNetTestSuite))
}

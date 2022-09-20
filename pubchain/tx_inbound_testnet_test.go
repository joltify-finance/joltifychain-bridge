package pubchain

import (
	"encoding/hex"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
	"gitlab.com/oppy-finance/oppy-bridge/tokenlist"
	"math/big"
	"sync"
	"testing"
)

type TestNetTestSuite struct {
	suite.Suite
	pubChain *Instance
	pk1      string
	sk1      *secp256k1.PrivKey
	pk2      string
	sk2      *secp256k1.PrivKey
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

//TODO need to update the tx as our bsc node is the pruned node
func (tn TestNetTestSuite) TestCheckTxStatus() {
	goodTx := "0x1ac52646d3fcb34114b08ef78e5f9de4eddff79ecbfee969c1caeb92b8282139"
	err := tn.pubChain.CheckTxStatus(goodTx)
	tn.Require().NoError(err)
	errorTx := "0x0361da29d01316131eb957fea3b90f55ca486cedb0ddb3c28442517b341351e8"
	err = tn.pubChain.CheckTxStatus(errorTx)
	tn.Require().Error(err)
}

// func (tn TestNetTestSuite) TestDoMoveFund() {
//	oppyAddr, err := misc.PoolPubKeyToOppyAddress(tn.pk1)
//	tn.Require().NoError(err)
//	ea, err := misc.PoolPubKeyToEthAddress(tn.pk1)
//	tn.Require().NoError(err)
//
//	pool := bcommon.PoolInfo{
//		Pk:          tn.pk1,
//		OppyAddress: oppyAddr,
//		EthAddress:  ea,
//	}
//
//	oppyAddr2, err := misc.PoolPubKeyToOppyAddress(tn.pk2)
//	tn.Require().NoError(err)
//	ea2, err := misc.PoolPubKeyToEthAddress(tn.pk2)
//	tn.Require().NoError(err)
//
//	pool2 := bcommon.PoolInfo{
//		Pk:          tn.pk2,
//		OppyAddress: oppyAddr2,
//		EthAddress:  ea2,
//	}
//
//	poolInfo1 := vaulttypes.PoolInfo{
//		BlockHeight: "100",
//		CreatePool: &vaulttypes.PoolProposal{
//			PoolPubKey: tn.pk1,
//			Nodes:      nil,
//		},
//	}
//
//	poolInfo2 := vaulttypes.PoolInfo{
//		BlockHeight: "100",
//		CreatePool: &vaulttypes.PoolProposal{
//			PoolPubKey: tn.pk2,
//			Nodes:      nil,
//		},
//	}
//
//	err = tn.pubChain.UpdatePool(&poolInfo2)
//	tn.Require().NoError(err)
//	err = tn.pubChain.UpdatePool(&poolInfo2)
//	tn.Require().NoError(err)
//
//	// if the  test fail, you need to disable the check for pool1->pool2 and
//	// run the test once to allow the fund move from pool2 back to pool1
//	// currently, we return false to enable double check of ERC20 token transfer
//	ret := tn.pubChain.MoveFound(100, &pool, tn.pubChain.EthClient)
//	time.Sleep(time.Second * 10)
//	// as we always retry, so we get false fir the first time we run the check
//	tn.Require().False(ret)
//	ret = tn.pubChain.MoveFound(100, &pool, tn.pubChain.EthClient)
//	tn.Require().True(ret)
//
//	tn.pubChain.MoveFound(100, &pool, tn.pubChain.EthClient)
//	time.Sleep(time.Second * 10)
//	tn.Require().True(ret)
//
//	// move token back
//	tn.pubChain.tssServer = &TssMock{sk: tn.sk2}
//	err = tn.pubChain.UpdatePool(&poolInfo1)
//	tn.Require().NoError(err)
//	err = tn.pubChain.UpdatePool(&poolInfo1)
//	tn.Require().NoError(err)
//
//	ret = tn.pubChain.MoveFound(100, &pool2, tn.pubChain.EthClient)
//	time.Sleep(time.Second * 10)
//	tn.Require().False(ret)
//
//	ret = tn.pubChain.MoveFound(100, &pool2, tn.pubChain.EthClient)
//	tn.Require().True(ret)
//
//	ret = tn.pubChain.MoveFound(100, &pool2, tn.pubChain.EthClient)
//	tn.Require().True(ret)
//}

func TestEvent(t *testing.T) {
	suite.Run(t, new(TestNetTestSuite))
}

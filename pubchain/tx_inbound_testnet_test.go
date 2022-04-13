package pubchain

import (
	"encoding/hex"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32"
	"github.com/stretchr/testify/suite"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
	"math/big"
	"sync"
	"testing"
	"time"
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
	websocketTest := "ws://10.2.118.8:8456/"
	tokenAddrTest := "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"

	a1 := "c225ac7cf268405670c004e0b8f6b7df5fefb80f3505aaf9619ea89c787a67e7"
	a2 := "481d305c7be328b6defd500209f9fdfb5447231f4c1f665324df951029506e12"
	data, err := hex.DecodeString(a1)
	if err != nil {
		panic(err)
	}
	sk := secp256k1.PrivKey{Key: data}
	pk, err := legacybech32.MarshalPubKey(legacybech32.AccPK, sk.PubKey())
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
	pk2, err := legacybech32.MarshalPubKey(legacybech32.AccPK, sk2.PubKey())
	if err != nil {
		panic(err)
	}

	tn.pk2 = pk2
	tn.sk2 = &sk2

	tss := TssMock{sk: &sk}
	pubChain, err := NewChainInstance(websocketTest, tokenAddrTest, &tss)
	if err != nil {
		panic(err)
	}

	tn.pubChain = pubChain

}

func (tn TestNetTestSuite) TestProcessNewBlock() {

	err := tn.pubChain.ProcessNewBlock(big.NewInt(18427951), 100)
	tn.Require().NoError(err)
}

func (tn TestNetTestSuite) TestCheckTxStatus() {

	goodTx := "0xf579e87bbeba30dda2a183e9df0dde41df7f980bea861a340546b06ff17110d9"
	err := tn.pubChain.CheckTxStatus(goodTx)
	tn.Require().NoError(err)
	errorTx := "0x26e21b05ba592eddf290ddeb39fa3f4ed44227854ae42b0b47ee880a973b683f"
	err = tn.pubChain.CheckTxStatus(errorTx)
	tn.Require().Error(err)
}

func (tn TestNetTestSuite) TestDoMoveFund() {

	wg := sync.WaitGroup{}
	ja, err := misc.PoolPubKeyToJoltAddress(tn.pk1)
	tn.Require().NoError(err)
	ea, err := misc.PoolPubKeyToEthAddress(tn.pk1)
	tn.Require().NoError(err)

	pool := bcommon.PoolInfo{
		Pk:             tn.pk1,
		JoltifyAddress: ja,
		EthAddress:     ea,
	}

	ja2, err := misc.PoolPubKeyToJoltAddress(tn.pk2)
	tn.Require().NoError(err)
	ea2, err := misc.PoolPubKeyToEthAddress(tn.pk2)
	tn.Require().NoError(err)

	pool2 := bcommon.PoolInfo{
		Pk:             tn.pk2,
		JoltifyAddress: ja2,
		EthAddress:     ea2,
	}
	_ = pool2
	_ = pool
	//_, err = tn.pubChain.doMoveFunds(&wg, &pool, ea2, 100)
	//tn.Require().NoError(err)
	//time.Sleep(time.Second * 10)
	//_, err = tn.pubChain.doMoveFunds(&wg, &pool, ea2, 101)
	//tn.Require().NoError(err)
	//// we transfer the money back
	//tn.pubChain.tssServer = &TssMock{sk: tn.sk2}
	//
	//time.Sleep(time.Second * 10)
	//_, err = tn.pubChain.doMoveFunds(&wg, &pool2, ea, 100)
	//tn.Require().NoError(err)
	//time.Sleep(time.Second * 10)
	//_, err = tn.pubChain.doMoveFunds(&wg, &pool2, ea, 101)
	//tn.Require().NoError(err)
	//
	//wg.Wait()

	poolInfo1 := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: tn.pk1,
			Nodes:      nil,
		},
	}

	poolInfo2 := vaulttypes.PoolInfo{
		BlockHeight: "100",
		CreatePool: &vaulttypes.PoolProposal{
			PoolPubKey: tn.pk2,
			Nodes:      nil,
		},
	}

	tn.pubChain.UpdatePool(&poolInfo2)
	tn.pubChain.UpdatePool(&poolInfo2)

	ret := tn.pubChain.MoveFound(&wg, 100, &pool)
	time.Sleep(time.Second * 10)
	tn.Require().True(ret)
	tn.pubChain.MoveFound(&wg, 100, &pool)
	time.Sleep(time.Second * 10)
	tn.Require().True(ret)

	tn.pubChain.MoveFound(&wg, 100, &pool)
	time.Sleep(time.Second * 10)
	tn.Require().True(ret)

	tn.pubChain.tssServer = &TssMock{sk: tn.sk2}
	tn.pubChain.UpdatePool(&poolInfo1)
	tn.pubChain.UpdatePool(&poolInfo1)

	ret = tn.pubChain.MoveFound(&wg, 100, &pool2)
	time.Sleep(time.Second * 10)
	tn.Require().True(ret)
	tn.pubChain.MoveFound(&wg, 100, &pool2)
	time.Sleep(time.Second * 10)
	tn.Require().True(ret)
	wg.Wait()
}

func TestEvent(t *testing.T) {
	suite.Run(t, new(TestNetTestSuite))
}
package storage

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

type FileStateMgrTestSuite struct{ suite.Suite }

func (s *FileStateMgrTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
}

func createdTestOutBoundReqs(n int) []*common.OutBoundReq {
	retReq := make([]*common.OutBoundReq, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testCoin := sdk.NewCoin("test", sdk.NewInt(32))
		sk, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		addr := crypto.PubkeyToAddress(sk.PublicKey)
		item := common.NewOutboundReq(txid, addr, addr, testCoin, "testTokenAddr", int64(i), nil, nil)
		retReq[i] = &item
	}
	return retReq
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
		item := common.NewAccountInboundReq(accs[i].Address, addr, testCoin, []byte(txid), int64(i))
		retReq[i] = &item
	}
	return retReq
}

func (s *FileStateMgrTestSuite) TestSaveOutBoundState() {
	folder := os.TempDir()
	fileName := path.Join(folder, "outboundtx.dat")
	defer func() {
		err := os.RemoveAll(fileName)
		s.Require().NoError(err)
	}()
	fsm := NewTxStateMgr(folder)
	s.Require().NotNil(fsm)

	testReqs := createdTestOutBoundReqs(100)

	err := fsm.SaveOutBoundState(testReqs[:50])
	s.Require().NoError(err)

	loaded, err := fsm.LoadOutBoundState()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := testReqs[i].TxID
		s.Require().Equal(expectedTx, loadedTx)
	}

	err = fsm.SaveOutBoundState(testReqs[50:])
	s.Require().NoError(err)

	loaded, err = fsm.LoadOutBoundState()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := testReqs[50+i].TxID
		s.Require().Equal(expectedTx, loadedTx)
	}
}

func (s *FileStateMgrTestSuite) TestSaveInBoundState() {
	folder := os.TempDir()
	fileName := path.Join(folder, "inboundtx.dat")
	defer func() {
		err := os.RemoveAll(fileName)
		s.Require().NoError(err)
	}()
	fsm := NewTxStateMgr(folder)
	s.Require().NotNil(fsm)

	testReqs := createdTestInBoundReqs(100)

	err := fsm.SaveInBoundState(testReqs[:50])
	s.Require().NoError(err)

	loaded, _ := fsm.LoadInBoundState()
	s.Require().NotNil(loaded)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := testReqs[i].TxID
		s.Require().True(bytes.Equal(loadedTx, expectedTx))
	}

	err = fsm.SaveInBoundState(testReqs[50:])
	s.Require().NoError(err)

	loaded, err = fsm.LoadInBoundState()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := testReqs[50+i].TxID
		s.Require().True(bytes.Equal(loadedTx, expectedTx))
	}
}

func TestEvent(t *testing.T) {
	suite.Run(t, new(FileStateMgrTestSuite))
}

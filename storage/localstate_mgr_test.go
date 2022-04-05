package storage

import (
	"bytes"
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/ethereum/go-ethereum/crypto"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	. "gopkg.in/check.v1"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"
)

type FileStateMgrTestSuite struct{}

var _ = Suite(&FileStateMgrTestSuite{})

func TestPackage(t *testing.T) { TestingT(t) }

func (s *FileStateMgrTestSuite) SetUpTest(c *C) {
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
		item := common.NewOutboundReq(txid, addr, addr, testCoin, int64(i), int64(i))
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
		item := common.NewAccountInboundReq(accs[i].Address, addr, testCoin, []byte(txid), int64(i), int64(i))
		retReq[i] = &item
	}
	return retReq
}

func (s *FileStateMgrTestSuite) TestSaveOutBoundState(c *C) {
	folder := os.TempDir()
	fileName := path.Join(folder, "outboundtx.dat")
	defer func() {
		err := os.RemoveAll(fileName)
		c.Assert(err, IsNil)
	}()
	fsm := NewTxStateMgr(folder)
	c.Assert(fsm, NotNil)

	testReqs := createdTestOutBoundReqs(100)

	err := fsm.SaveOutBoundState(testReqs[:50])
	c.Assert(err, IsNil)

	loaded, err := fsm.LoadOutBoundState()
	c.Assert(err, IsNil)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := testReqs[i].TxID
		c.Assert(loadedTx, Equals, expectedTx)
	}

	err = fsm.SaveOutBoundState(testReqs[50:])
	c.Assert(err, IsNil)

	loaded, err = fsm.LoadOutBoundState()
	c.Assert(err, IsNil)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := testReqs[50+i].TxID
		c.Assert(loadedTx, Equals, expectedTx)
	}

}

func (s *FileStateMgrTestSuite) TestSaveInBoundState(c *C) {
	folder := os.TempDir()
	fileName := path.Join(folder, "inboundtx.dat")
	defer func() {
		err := os.RemoveAll(fileName)
		c.Assert(err, IsNil)
	}()
	fsm := NewTxStateMgr(folder)
	c.Assert(fsm, NotNil)

	testReqs := createdTestInBoundReqs(100)

	err := fsm.SaveInBoundState(testReqs[:50])
	c.Assert(err, IsNil)

	loaded, err := fsm.LoadInBoundState()
	c.Assert(err, IsNil)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := testReqs[i].TxID
		c.Assert(bytes.Equal(loadedTx, expectedTx), Equals, true)
	}

	err = fsm.SaveInBoundState(testReqs[50:])
	c.Assert(err, IsNil)

	loaded, err = fsm.LoadInBoundState()
	c.Assert(err, IsNil)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := testReqs[50+i].TxID
		c.Assert(bytes.Equal(loadedTx, expectedTx), Equals, true)
	}

}

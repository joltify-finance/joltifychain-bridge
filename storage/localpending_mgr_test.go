package storage

import (
	"fmt"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
)

type PendingFileStateMgrTestSuite struct{ suite.Suite }

func (s *PendingFileStateMgrTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
}

func createdTestPendingTxs(n int) []*pubchain.InboundTx {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	accs := simulation.RandomAccounts(r, n)
	pendingTxs := make([]*pubchain.InboundTx, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testToken := sdk.NewCoin("testToken", sdk.NewInt(32))
		testFee := sdk.NewCoin("testFee", sdk.NewInt(32))
		tx := pubchain.InboundTx{
			TxID:           txid,
			Address:        accs[i].Address,
			PubBlockHeight: uint64(i),
			Token:          testToken,
			Fee:            testFee,
		}
		pendingTxs[i] = &tx
	}
	return pendingTxs
}

func createdTestPendingBnbTxs(n int) []*pubchain.InboundTxBnb {
	pendingTxs := make([]*pubchain.InboundTxBnb, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testFee := sdk.NewCoin("testFee", sdk.NewInt(32))
		bnbtx := pubchain.InboundTxBnb{
			TxID:        txid,
			BlockHeight: uint64(i),
			Fee:         testFee,
		}
		pendingTxs[i] = &bnbtx
	}
	return pendingTxs
}

func (s *PendingFileStateMgrTestSuite) TestSavePendingItems() {
	folder := os.TempDir()
	fileName := path.Join(folder, "inboundpendingtx.dat")
	defer func() {
		err := os.RemoveAll(fileName)
		s.Require().NoError(err)
	}()
	pfsm := NewPendingTxStateMgr(folder)
	s.Require().NotNil(pfsm)

	pendingTxs := createdTestPendingTxs(100)

	err := pfsm.SavePendingItems(pendingTxs[:50])
	s.Require().NoError(err)

	loaded, err := pfsm.LoadPendingItems()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := pendingTxs[i].TxID
		s.Require().Equal(expectedTx, loadedTx)
	}

	err = pfsm.SavePendingItems(pendingTxs[50:])
	s.Require().NoError(err)

	loaded, err = pfsm.LoadPendingItems()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := pendingTxs[50+i].TxID
		s.Require().Equal(expectedTx, loadedTx)
	}
}

func (s *PendingFileStateMgrTestSuite) TestSavePendingBnbItems() {
	folder := os.TempDir()
	fileName := path.Join(folder, "inboundpendingbnbtx.dat")
	defer func() {
		err := os.RemoveAll(fileName)
		s.Require().NoError(err)
	}()
	pfsm := NewPendingTxStateMgr(folder)
	s.Require().NotNil(pfsm)

	pendingBnbTxs := createdTestPendingBnbTxs(100)

	err := pfsm.SavePendingBnbItems(pendingBnbTxs[:50])
	s.Require().NoError(err)

	loaded, err := pfsm.LoadPendingBnbItems()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := pendingBnbTxs[i].TxID
		s.Require().Equal(expectedTx, loadedTx)
	}

	err = pfsm.SavePendingBnbItems(pendingBnbTxs[50:])
	s.Require().NoError(err)

	loaded, err = pfsm.LoadPendingBnbItems()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].TxID
		expectedTx := pendingBnbTxs[50+i].TxID
		s.Require().Equal(expectedTx, loadedTx)
	}
}

func TestPendingEvent(t *testing.T) {
	suite.Run(t, new(PendingFileStateMgrTestSuite))
}

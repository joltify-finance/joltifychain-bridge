package storage

import (
	"fmt"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/oppy-finance/oppy-bridge/oppybridge"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/stretchr/testify/suite"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

type PendingFileStateMgrTestSuite struct{ suite.Suite }

func (s *PendingFileStateMgrTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
}

func createdTestPendingTxs(n int) []*oppybridge.OutboundTx {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	accs := simulation.RandomAccounts(r, n)
	pendingTxs := make([]*oppybridge.OutboundTx, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testToken := sdk.NewCoin("testToken", sdk.NewInt(32))
		testFee := sdk.NewCoin("testFee", sdk.NewInt(32))
		tx := oppybridge.OutboundTx{
			TxID:               txid,
			OutReceiverAddress: common.HexToAddress(accs[i].Address.String()),
			BlockHeight:        uint64(i),
			Token:              testToken,
			Fee:                testFee,
			TokenAddr:          "testAddress",
		}
		pendingTxs[i] = &tx
	}
	return pendingTxs
}

func (s *PendingFileStateMgrTestSuite) TestSavePendingItems() {
	folder := os.TempDir()
	fileName := path.Join(folder, "outboundpendingtx.dat")
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

func TestPendingEvent(t *testing.T) {
	suite.Run(t, new(PendingFileStateMgrTestSuite))
}

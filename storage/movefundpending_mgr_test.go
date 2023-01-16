package storage

import (
	"os"
	"path"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

type PendingMoveFundMgrTestSuite struct{ suite.Suite }

func (s *PendingMoveFundMgrTestSuite) SetupSuite() {
	misc.SetupBech32Prefix()
}

func createdTestMoveFundItems(n int) []*common.MoveFundItem {
	retReq := make([]*common.MoveFundItem, n)
	for i := 0; i < n; i++ {
		sk, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		addr := crypto.PubkeyToAddress(sk.PublicKey)
		item := common.PoolInfo{
			EthAddress: addr,
		}
		retReq[i] = &common.MoveFundItem{
			PoolInfo:  &item,
			ChainType: "BSC",
			Height:    int64(i),
		}
	}
	return retReq
}

func (s *PendingMoveFundMgrTestSuite) TestSaveOutBoundState() {
	folder := os.TempDir()
	fileName := path.Join(folder, "outboundtx.dat")
	defer func() {
		err := os.RemoveAll(fileName)
		s.Require().NoError(err)
	}()
	fsm := NewMoveFundStateMgr(folder)
	s.Require().NotNil(fsm)

	testReqs := createdTestMoveFundItems(100)

	err := fsm.SavePendingItems(testReqs[:50])
	s.Require().NoError(err)

	loaded, err := fsm.LoadPendingItems()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].Height
		expectedTx := testReqs[i].Height
		s.Require().Equal(expectedTx, loadedTx)
	}

	err = fsm.SavePendingItems(testReqs[50:])
	s.Require().NoError(err)

	loaded, err = fsm.LoadPendingItems()
	s.Require().NoError(err)

	for i := 0; i < 50; i++ {
		loadedTx := loaded[i].Height
		expectedTx := testReqs[50+i].Height
		s.Require().Equal(expectedTx, loadedTx)
	}
}

func TestMovingFundEvent(t *testing.T) {
	suite.Run(t, new(PendingMoveFundMgrTestSuite))
}

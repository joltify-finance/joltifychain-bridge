package pubchain

import (
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/misc"
)

func createdTestOutBoundReqs(n int) []*common.InBoundReq {
	retReq := make([]*common.InBoundReq, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testCoin := sdk.NewCoin("test", sdk.NewInt(32))
		sk, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		addr := crypto.PubkeyToAddress(sk.PublicKey)
		joltAddress := "oppy1rfmwldwrm3652shx3a7say0v4vvtglasncg0uu"
		jaddr, err := sdk.AccAddressFromBech32(joltAddress)
		if err != nil {
			panic(err)
		}
		item := common.NewAccountInboundReq(jaddr, addr, testCoin, []byte(txid), int64(i), int64(i))
		retReq[i] = &item
	}
	return retReq
}

func TestConfig(t *testing.T) {
	misc.SetupBech32Prefix()
	reqs := createdTestOutBoundReqs(100)
	jc := Instance{
		RetryInboundReq: &sync.Map{},
		InboundReqChan:  make(chan *common.InBoundReq, 10),
		moveFundReq:     &sync.Map{},
	}

	for _, el := range reqs {
		jc.AddItem(el)
	}
	assert.Equal(t, jc.Size(), 100)

	poped := jc.PopItem(1000)
	assert.Equal(t, 100, len(poped))
	var allindex []*big.Int
	for _, el := range poped {
		allindex = append(allindex, el.Index())
	}
	//now we check it is sorted
	for i := 1; i < len(poped); i++ {
		assert.Equal(t, allindex[i].Cmp(allindex[i-1]), 1)
	}

	assert.Equal(t, jc.Size(), 0)

	for _, el := range reqs {
		jc.AddItem(el)
	}
	item := jc.ExportItems()
	assert.Equal(t, len(item), 100)

	accs, err := generateRandomPrivKey(2)
	assert.Nil(t, err)
	pool := common.PoolInfo{
		Pk:          accs[0].pk,
		OppyAddress: accs[0].joltAddr,
		EthAddress:  accs[0].commAddr,
	}

	pool1 := common.PoolInfo{
		Pk:          accs[1].pk,
		OppyAddress: accs[1].joltAddr,
		EthAddress:  accs[1].commAddr,
	}

	jc.AddMoveFundItem(&pool, 10)
	jc.AddMoveFundItem(&pool1, 11)
	popedItem, _ := jc.PopMoveFundItemAfterBlock(15)
	assert.Nil(t, popedItem)

	popedItem, _ = jc.PopMoveFundItemAfterBlock(20)
	assert.Equal(t, popedItem.Pk, accs[0].pk)
}

func createdTestPendingTxs(n int) []*InboundTx {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	accs := simulation.RandomAccounts(r, n)
	pendingTxs := make([]*InboundTx, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testToken := sdk.NewCoin("testToken", sdk.NewInt(32))
		testFee := sdk.NewCoin("testFee", sdk.NewInt(32))
		tx := InboundTx{
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

func createdTestPendingBnbTxs(n int) []*InboundTxBnb {
	pendingTxs := make([]*InboundTxBnb, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testFee := sdk.NewCoin("testFee", sdk.NewInt(32))
		bnbtx := InboundTxBnb{
			TxID:        txid,
			BlockHeight: uint64(i),
			Fee:         testFee,
		}
		pendingTxs[i] = &bnbtx
	}
	return pendingTxs
}

func TestPendingConfig(t *testing.T) {
	misc.SetupBech32Prefix()
	pendingTxs := createdTestPendingTxs(100)
	pendingBnbTxs := createdTestPendingBnbTxs(100)
	pi := Instance{
		pendingInbounds:    &sync.Map{},
		pendingInboundsBnB: &sync.Map{},
	}

	// testing for pending tx
	for _, el := range pendingTxs {
		pi.AddPendingTx(el)
	}
	var beforeAddTxSize int
	pi.pendingInbounds.Range(func(k, v interface{}) bool {
		beforeAddTxSize++
		return true
	})
	assert.Equal(t, beforeAddTxSize, 100)

	exportedPendingTxs := pi.ExportPendingItems()
	assert.Equal(t, len(exportedPendingTxs), 100)

	// testing for pending bnb tx
	for _, el := range pendingBnbTxs {
		pi.AddPendingTxBnb(el)
	}
	var beforeAddBnbTxSize int
	pi.pendingInboundsBnB.Range(func(k, v interface{}) bool {
		beforeAddBnbTxSize++
		return true
	})
	assert.Equal(t, beforeAddBnbTxSize, 100)

	exportedPendingBnbTxs := pi.ExportPendingBnbItems()
	assert.Equal(t, len(exportedPendingBnbTxs), 100)
}

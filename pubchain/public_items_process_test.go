package pubchain

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
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
		oppyAddress := "oppy1rfmwldwrm3652shx3a7say0v4vvtglasncg0uu"
		oaddr, err := sdk.AccAddressFromBech32(oppyAddress)
		if err != nil {
			panic(err)
		}
		item := common.NewAccountInboundReq(oaddr, addr, testCoin, []byte(txid), int64(i))
		retReq[i] = &item
	}
	return retReq
}

func TestConfig(t *testing.T) {
	misc.SetupBech32Prefix()
	reqs := createdTestOutBoundReqs(100)
	oc := Instance{
		RetryInboundReq:      &sync.Map{},
		InboundReqChan:       make(chan []*common.InBoundReq, 10),
		moveFundReq:          &sync.Map{},
		onHoldRetryQueueLock: &sync.Mutex{},
		onHoldRetryQueue:     []*common.InBoundReq{},
	}

	for _, el := range reqs {
		oc.AddItem(el)
	}
	assert.Equal(t, oc.Size(), 100)

	poped := oc.PopItem(1000)
	assert.Equal(t, 100, len(poped))
	allIndex := make([]string, len(poped))
	for i, el := range poped {
		allIndex[i] = el.Index()
	}
	// now we check it is sorted
	for i := 1; i < len(poped); i++ {
		a, ok := new(big.Int).SetString(allIndex[i], 10)
		assert.True(t, ok)
		b, ok := new(big.Int).SetString(allIndex[i-1], 10)
		assert.True(t, ok)
		assert.True(t, a.Cmp(b) == 1)
	}

	assert.Equal(t, oc.Size(), 0)

	for _, el := range reqs {
		oc.AddItem(el)
	}
	item := oc.ExportItems()
	assert.Equal(t, len(item), 100)

	ret := oc.IsEmpty()
	assert.False(t, ret)

	accs, err := generateRandomPrivKey(2)
	assert.Nil(t, err)
	pool := common.PoolInfo{
		Pk:          accs[0].pk,
		OppyAddress: accs[0].oppyAddr,
		EthAddress:  accs[0].commAddr,
	}

	pool1 := common.PoolInfo{
		Pk:          accs[1].pk,
		OppyAddress: accs[1].oppyAddr,
		EthAddress:  accs[1].commAddr,
	}

	oc.AddMoveFundItem(&pool, 10)
	oc.AddMoveFundItem(&pool1, 11)
	popedItem, _ := oc.PopMoveFundItemAfterBlock(15)
	assert.Nil(t, popedItem)
	popedItem, _ = oc.PopMoveFundItemAfterBlock(2000)
	assert.Equal(t, popedItem.Pk, accs[0].pk)

	for _, el := range reqs {
		oc.AddOnHoldQueue(el)
	}
	assert.Equal(t, len(oc.onHoldRetryQueue), len(reqs))
	exported := oc.ExportMoveFundItems()
	assert.Equal(t, len(exported), 1)

	dumpped := oc.DumpQueue()
	assert.Equal(t, len(dumpped), len(reqs))
	assert.Equal(t, len(oc.onHoldRetryQueue), 0)
}

package oppybridge

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"gitlab.com/oppy-finance/oppy-bridge/common"
)

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
		item := common.NewOutboundReq(txid, addr, addr, testCoin, "testAddr", int64(i), int64(i))
		retReq[i] = &item
	}
	return retReq
}

func TestConfig(t *testing.T) {
	reqs := createdTestOutBoundReqs(100)

	oc := OppyChainInstance{
		RetryOutboundReq: &sync.Map{},
		OutboundReqChan:  make(chan *common.OutBoundReq, 10),
		moveFundReq:      &sync.Map{},
	}

	for _, el := range reqs {
		oc.AddItem(el)
	}
	assert.Equal(t, oc.Size(), 100)

	poped := oc.PopItem(1000)
	assert.Equal(t, 100, len(poped))
	var allindex []*big.Int
	for _, el := range poped {
		allindex = append(allindex, el.Index())
	}
	//now we check it is sorted
	for i := 1; i < len(poped); i++ {
		assert.Equal(t, allindex[i].Cmp(allindex[i-1]), 1)
	}

	assert.Equal(t, oc.Size(), 0)

	for _, el := range reqs {
		oc.AddItem(el)
	}
	item := oc.ExportItems()
	assert.Equal(t, len(item), 100)

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
	popedItem, _ := oc.popMoveFundItemAfterBlock(15)
	assert.Nil(t, popedItem)

	popedItem, _ = oc.popMoveFundItemAfterBlock(20)
	assert.Equal(t, popedItem.Pk, accs[0].pk)
}

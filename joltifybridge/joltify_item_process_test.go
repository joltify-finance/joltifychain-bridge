package joltifybridge

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"gitlab.com/joltify/joltifychain-bridge/common"
)

// func createdTestJoltTokenList() JoltifyTokenList {
// 	tokenList := new(sync.Map)
// 	tokenList.Store("testDenom", "testAddr")
// 	return JoltifyTokenList{
// 		CurrentTokenlist: tokenList,
// 	}
// }

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

	jc := JoltifyChainInstance{
		RetryOutboundReq: &sync.Map{},
		OutboundReqChan:  make(chan *common.OutBoundReq, 10),
		moveFundReq:      &sync.Map{},
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
		Pk:             accs[0].pk,
		JoltifyAddress: accs[0].joltAddr,
		EthAddress:     accs[0].commAddr,
	}

	pool1 := common.PoolInfo{
		Pk:             accs[1].pk,
		JoltifyAddress: accs[1].joltAddr,
		EthAddress:     accs[1].commAddr,
	}

	jc.AddMoveFundItem(&pool, 10)
	jc.AddMoveFundItem(&pool1, 11)
	popedItem, _ := jc.popMoveFundItemAfterBlock(15)
	assert.Nil(t, popedItem)

	popedItem, _ = jc.popMoveFundItemAfterBlock(20)
	assert.Equal(t, popedItem.Pk, accs[0].pk)
}

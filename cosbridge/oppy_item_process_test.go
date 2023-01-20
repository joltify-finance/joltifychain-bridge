package cosbridge

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"gitlab.com/joltify/joltifychain-bridge/common"
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
		item := common.NewOutboundReq(hex.EncodeToString([]byte(txid)), addr.Bytes(), addr.Bytes(), testCoin, "testAddr", int64(i), nil, "BSC", true)
		retReq[i] = &item
	}
	return retReq
}

func TestConfig(t *testing.T) {
	reqs := createdTestOutBoundReqs(100)

	oc := JoltChainInstance{
		RetryOutboundReq: &sync.Map{},
		OutboundReqChan:  make(chan []*common.OutBoundReq, 10),
	}

	for _, el := range reqs {
		oc.AddItem(el)
	}
	assert.Equal(t, oc.Size(), 100)

	poped := oc.PopItem(1000, "BSC")
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
}

func TestAddAndDumpQueue(t *testing.T) {
	reqs := createdTestOutBoundReqs(100)

	oc := JoltChainInstance{
		RetryOutboundReq:     &sync.Map{},
		OutboundReqChan:      make(chan []*common.OutBoundReq, 10),
		onHoldRetryQueue:     []*common.OutBoundReq{},
		onHoldRetryQueueLock: &sync.Mutex{},
	}

	for _, el := range reqs {
		oc.AddOnHoldQueue(el)
	}

	outputQueue := oc.DumpQueue()
	assert.Equal(t, len(outputQueue), 100)
	assert.Equal(t, len(oc.onHoldRetryQueue), 0)
	outputQueue = oc.DumpQueue()
	assert.Equal(t, len(outputQueue), 0)
}

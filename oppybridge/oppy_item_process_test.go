package oppybridge

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/types/simulation"
	ethcommon "github.com/ethereum/go-ethereum/common"

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
		item := common.NewOutboundReq(hex.EncodeToString([]byte(txid)), addr, addr, testCoin, "testAddr", int64(i), nil, nil)
		retReq[i] = &item
	}
	return retReq
}

func TestConfig(t *testing.T) {
	reqs := createdTestOutBoundReqs(100)

	oc := OppyChainInstance{
		RetryOutboundReq: &sync.Map{},
		OutboundReqChan:  make(chan []*common.OutBoundReq, 10),
		pendingTx:        &sync.Map{},
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

	// we test imported data
	data := createdTestPendingTxs(100)
	oc.Import(data)
	saved := make([]*OutboundTx, len(data))

	for i, el := range data {
		dat, ok := oc.pendingTx.Load(el.TxID)
		assert.Equal(t, ok, true)
		saved[i] = dat.(*OutboundTx)
	}

	for i := 0; i < 100; i++ {
		assert.Equal(t, saved[i].OutReceiverAddress.String(), data[i].OutReceiverAddress.String())
		assert.True(t, saved[i].Fee.IsEqual(data[i].Fee))
		assert.Equal(t, saved[i].BlockHeight, data[i].BlockHeight)
	}

	exportedData := oc.Export()

	sort.Slice(exportedData, func(i, j int) bool {
		return exportedData[i].TxID < exportedData[j].TxID
	})

	sort.Slice(data, func(i, j int) bool {
		return data[i].TxID < data[j].TxID
	})

	for i := 0; i < 100; i++ {
		assert.Equal(t, exportedData[i].OutReceiverAddress.String(), data[i].OutReceiverAddress.String())
		assert.True(t, exportedData[i].Fee.IsEqual(data[i].Fee))
		assert.Equal(t, exportedData[i].BlockHeight, data[i].BlockHeight)
	}
}

func createdTestPendingTxs(n int) []*OutboundTx {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	accs := simulation.RandomAccounts(r, n)
	pendingTxs := make([]*OutboundTx, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testToken := sdk.NewCoin("testToken", sdk.NewInt(32))
		testFee := sdk.NewCoin("testFee", sdk.NewInt(32))
		tx := OutboundTx{
			TxID:               txid,
			OutReceiverAddress: ethcommon.HexToAddress(accs[i].Address.String()),
			BlockHeight:        uint64(i),
			Token:              testToken,
			Fee:                testFee,
			TokenAddr:          "testAddress",
		}
		pendingTxs[i] = &tx
	}
	return pendingTxs
}

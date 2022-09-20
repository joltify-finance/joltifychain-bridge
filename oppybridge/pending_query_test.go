package oppybridge

import (
	"fmt"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
)

func TestQueryPending(t *testing.T) {
	oc := OppyChainInstance{
		pendingTx: &sync.Map{},
	}

	fee := sdk.NewCoin("test", sdk.NewInt(1))
	feeWanted := sdk.NewCoin("test", sdk.NewInt(10))
	for i := 0; i < 100; i++ {
		from := fmt.Sprintf("tester%v", i%10)
		txID := fmt.Sprintf("tx%v", i)
		tx := OutboundTx{
			TxID:        txID,
			FromAddress: from,
			Fee:         fee,
			FeeWanted:   feeWanted,
		}
		oc.pendingTx.Store(txID, &tx)
	}
	result := oc.QueryPendingTx("tester2")
	assert.Equal(t, len(result), 10)

	result = oc.QueryPendingTx("testerbad")
	assert.Equal(t, len(result), 0)
}

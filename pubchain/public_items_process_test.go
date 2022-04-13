package pubchain

import (
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/misc"
	"math/big"
	"sync"
	"testing"
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
		joltAddress := "jolt1sdw2qtv6nj5zwfje57rff0dlvr0hwezkqudn5w"
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
	popedItem, _ := jc.PopMoveFundItemAfterBlock(15)
	assert.Nil(t, popedItem)

	popedItem, _ = jc.PopMoveFundItemAfterBlock(20)
	assert.Equal(t, popedItem.Pk, accs[0].pk)
}

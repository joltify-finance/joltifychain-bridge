package common

import (
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"time"
)

func createdTestOutBoundReqs(n int) []*OutBoundReq {
	retReq := make([]*OutBoundReq, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testCoin := sdk.NewCoin("test", sdk.NewInt(32))
		sk, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		addr := crypto.PubkeyToAddress(sk.PublicKey)
		item := NewOutboundReq(txid, addr, addr, testCoin, int64(i), int64(i))
		retReq[i] = &item
	}
	return retReq
}

func createdTestInBoundReqs(n int) []*InBoundReq {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	accs := simulation.RandomAccounts(r, n)
	retReq := make([]*InBoundReq, n)
	for i := 0; i < n; i++ {
		txid := fmt.Sprintf("testTXID %v", i)
		testCoin := sdk.NewCoin("test", sdk.NewInt(32))
		sk, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		addr := crypto.PubkeyToAddress(sk.PublicKey)
		item := NewAccountInboundReq(accs[i].Address, addr, testCoin, []byte(txid), int64(i), int64(i))
		retReq[i] = &item
	}
	return retReq
}

func TestOutBoundTx(t *testing.T) {
	outboundreqs := createdTestOutBoundReqs(100)
	index := outboundreqs[0].Index()
	assert.NotNil(t, index)
	outboundreqs[0].SetItemHeightAndNonce(2, 100, 23)
	h := outboundreqs[0].Hash()
	assert.NotNil(t, h.Bytes())
	_, _, _, blockheight, nonce := outboundreqs[0].GetOutBoundInfo()
	assert.Equal(t, blockheight, int64(2))
	assert.Equal(t, nonce, uint64(23))
}

func TestInBoundTx(t *testing.T) {
	outboundreqs := createdTestInBoundReqs(2)
	index := outboundreqs[0].Index()
	assert.NotNil(t, index)
	outboundreqs[0].SetAccountInfo(2, 100, outboundreqs[1].PoolJoltifyAddress, "123")
	seq, num, _, _ := outboundreqs[0].GetAccountInfo()
	assert.Equal(t, seq, uint64(100))
	assert.Equal(t, num, uint64(2))
	outboundreqs[0].SetItemHeight(100)
	_, _, _, blockHeight := outboundreqs[0].GetInboundReqInfo()
	assert.Equal(t, blockHeight, int64(100))
}

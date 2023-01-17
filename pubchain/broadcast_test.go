package pubchain

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestBroadcast(t *testing.T) {
	brd := NewBroadcaster()
	outChains := make([]chan map[string][]byte, 3)
	for i := 0; i < 3; i++ {
		outChain, err := brd.Subscribe(int64(i))
		require.NoError(t, err)
		outChains[i] = outChain
	}

	wg := sync.WaitGroup{}
	wg.Add(3)
	counter := atomic.NewInt32(0)
	for i := 0; i < 3; i++ {
		go func(index int64) {
			defer wg.Done()
			<-brd.clients[index]
			counter.Inc()
		}(int64(i))
	}
	msgToBroadcast := make(map[string][]byte)
	msgToBroadcast["test"] = []byte("testme")
	brd.Broadcast(msgToBroadcast)
	wg.Wait()
	assert.Equal(t, counter.Load(), int32(3))
}

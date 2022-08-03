package pubchain

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testClient struct {
	name     string
	signal   <-chan struct{}
	signalID int64
	brd      *Broadcaster
}

func (c *testClient) doWork() {
	i := 0
	for range c.signal {
		fmt.Println(c.name, "do work", i)
		if i > 2 {
			c.brd.Unsubscribe(c.signalID)
			fmt.Println(c.name, "unsubscribed")
		}
		i++
	}
	fmt.Println(c.name, "done")
}

func TestBroadcast(t *testing.T) {
	var err error
	brd := NewBroadcaster()

	clients := make([]*testClient, 0)

	for i := 0; i < 3; i++ {
		c := &testClient{
			name:     fmt.Sprint("client:", i),
			signalID: time.Now().UnixNano() + int64(i), // +int64(i) for play.golang.org
			brd:      brd,
		}
		c.signal, err = brd.Subscribe(c.signalID)
		require.NoError(t, err)

		clients = append(clients, c)
	}

	for i := 0; i < len(clients); i++ {
		go clients[i].doWork()
	}

	for i := 0; i < 6; i++ {
		brd.Broadcast()
		time.Sleep(time.Second)
	}
}

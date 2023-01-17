package pubchain

import (
	"fmt"
	"sync"
)

type Broadcaster struct {
	mu      *sync.Mutex
	clients map[int64]chan map[string][]byte
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		mu:      &sync.Mutex{},
		clients: make(map[int64]chan map[string][]byte),
	}
}

func (b *Broadcaster) Subscribe(id int64) (chan map[string][]byte, error) {
	defer b.mu.Unlock()
	b.mu.Lock()
	s := make(chan map[string][]byte, 1)

	if _, ok := b.clients[id]; ok {
		return nil, fmt.Errorf("signal %d already exist", id)
	}

	b.clients[id] = s

	return b.clients[id], nil
}

func (b *Broadcaster) Unsubscribe(id int64) {
	defer b.mu.Unlock()
	b.mu.Lock()
	if _, ok := b.clients[id]; ok {
		close(b.clients[id])
	}

	delete(b.clients, id)
}

func (b *Broadcaster) Broadcast(ret map[string][]byte) {
	defer b.mu.Unlock()
	b.mu.Lock()
	for k := range b.clients {
		if len(b.clients[k]) == 0 {
			b.clients[k] <- ret
		}
	}
}

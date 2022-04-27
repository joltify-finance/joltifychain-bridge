package storage

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"sync"

	"gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/pubchain"
)

// LocalStateManager provide necessary methods to manage the local state, save it , and read it back
// LocalStateManager doesn't have any opinion in regards to where it should be persistent to
type LocalPendingTxManager interface {
	SaveOutBoundState([]*common.OutBoundReq) error
	LoadOutBoundState() []*common.OutBoundReq
	SaveInBoundState([]*common.InBoundReq) error
	LoadInBoundState() []*common.InBoundReq
}

// TxStateMgr save the local state to file
type PendingTxMgr struct {
	folder              string
	writePendingLock    *sync.RWMutex
	writePendingBnbLock *sync.RWMutex
	logger              zerolog.Logger
}

// NewTxStateMgr create a new instance of the FileStateMgr which implements LocalStateManager
func NewPendingTxStateMgr(folder string) *PendingTxMgr {
	logger := log.With().Str("module", "pubchain").Logger()
	return &PendingTxMgr{
		folder:              folder,
		writePendingLock:    &sync.RWMutex{},
		writePendingBnbLock: &sync.RWMutex{},
		logger:              logger,
	}
}

func (fsm *PendingTxMgr) SavePendingItems(pendingTxs []*pubchain.InboundTx) error {
	fsm.writePendingLock.Lock()
	defer fsm.writePendingLock.Unlock()

	filePathName := filepath.Join(fsm.folder, "inboundpendingtx.dat")
	buf, err := json.Marshal(pendingTxs)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to marshal the inbound pending tx")
		return err
	}
	return ioutil.WriteFile(filePathName, buf, 0o655)
}

func (fsm *PendingTxMgr) SavePendingBnbItems(pendingBnbTxs []*pubchain.InboundTxBnb) error {
	fsm.writePendingBnbLock.Lock()
	defer fsm.writePendingBnbLock.Unlock()

	filePathName := filepath.Join(fsm.folder, "inboundpendingbnbtx.dat")
	buf, err := json.Marshal(pendingBnbTxs)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to marshal the inbound pending bnb tx")
		return err
	}
	return ioutil.WriteFile(filePathName, buf, 0o655)
}

func (fsm *PendingTxMgr) LoadPendingItems() ([]*pubchain.InboundTx, error) {
	if len(fsm.folder) < 1 {
		return nil, errors.New("base file path is invalid")
	}
	filePathName := filepath.Join(fsm.folder, "inboundpendingtx.dat")
	_, err := os.Stat(filePathName)
	if err != nil {
		return nil, err
	}

	fsm.writePendingLock.RLock()
	input, err := ioutil.ReadFile(filePathName)
	if err != nil {
		fsm.writePendingLock.RUnlock()
		return nil, err
	}
	fsm.writePendingLock.RUnlock()
	var inBoundPendingTx []*pubchain.InboundTx
	err = json.Unmarshal(input, &inBoundPendingTx)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to unmarshal the inbound pending tx")
	}
	return inBoundPendingTx, err
}

func (fsm *PendingTxMgr) LoadPendingBnbItems() ([]*pubchain.InboundTxBnb, error) {
	if len(fsm.folder) < 1 {
		return nil, errors.New("base file path is invalid")
	}
	filePathName := filepath.Join(fsm.folder, "inboundpendingbnbtx.dat")
	_, err := os.Stat(filePathName)
	if err != nil {
		return nil, err
	}

	fsm.writePendingBnbLock.RLock()
	input, err := ioutil.ReadFile(filePathName)
	if err != nil {
		fsm.writePendingBnbLock.RUnlock()
		return nil, err
	}
	fsm.writePendingBnbLock.RUnlock()
	var inBoundPendingBnbTx []*pubchain.InboundTxBnb
	err = json.Unmarshal(input, &inBoundPendingBnbTx)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to unmarshal the inbound pending bnb tx")
	}
	return inBoundPendingBnbTx, err
}

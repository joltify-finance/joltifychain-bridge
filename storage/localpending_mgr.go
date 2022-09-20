package storage

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"gitlab.com/oppy-finance/oppy-bridge/oppybridge"
)

// PendingTxMgr save the local state to file
type PendingTxMgr struct {
	folder              string
	writePendingLock    *sync.RWMutex
	writePendingBnbLock *sync.RWMutex
	logger              zerolog.Logger
}

// NewPendingTxStateMgr create a new instance of the FileStateMgr which implements LocalStateManager
func NewPendingTxStateMgr(folder string) *PendingTxMgr {
	logger := log.With().Str("module", "peindingtxsave").Logger()
	return &PendingTxMgr{
		folder:              folder,
		writePendingLock:    &sync.RWMutex{},
		writePendingBnbLock: &sync.RWMutex{},
		logger:              logger,
	}
}

func (fsm *PendingTxMgr) SavePendingItems(pendingTxs []*oppybridge.OutboundTx) error {
	fsm.writePendingLock.Lock()
	defer fsm.writePendingLock.Unlock()

	filePathName := filepath.Join(fsm.folder, "outboundpendingtx.dat")
	buf, err := json.Marshal(pendingTxs)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to marshal the inbound pending tx")
		return err
	}
	return ioutil.WriteFile(filePathName, buf, 0o600)
}

func (fsm *PendingTxMgr) LoadPendingItems() ([]*oppybridge.OutboundTx, error) {
	if len(fsm.folder) < 1 {
		return nil, errors.New("base file path is invalid")
	}
	filePathName := filepath.Join(fsm.folder, "outboundpendingtx.dat")
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
	var outboundPending []*oppybridge.OutboundTx
	err = json.Unmarshal(input, &outboundPending)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to unmarshal the inbound pending tx")
	}
	return outboundPending, err
}

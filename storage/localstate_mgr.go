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
	"gitlab.com/oppy-finance/oppy-bridge/common"
)

// LocalStateManager provide necessary methods to manage the local state, save it , and read it back
// LocalStateManager doesn't have any opinion in regards to where it should be persistent to
type LocalStateManager interface {
	SaveOutBoundState([]*common.OutBoundReq) error
	LoadOutBoundState() []*common.OutBoundReq
	SaveInBoundState([]*common.InBoundReq) error
	LoadInBoundState() []*common.InBoundReq
}

// TxStateMgr save the local state to file
type TxStateMgr struct {
	folder            string
	writeOutBoundLock *sync.RWMutex
	writeInBoundLock  *sync.RWMutex
	logger            zerolog.Logger
}

// NewTxStateMgr create a new instance of the FileStateMgr which implements LocalStateManager
func NewTxStateMgr(folder string) *TxStateMgr {
	logger := log.With().Str("module", "tx save").Logger()
	return &TxStateMgr{
		folder:            folder,
		writeOutBoundLock: &sync.RWMutex{},
		writeInBoundLock:  &sync.RWMutex{},
		logger:            logger,
	}
}

func (fsm *TxStateMgr) SaveOutBoundState(reqs []*common.OutBoundReq) error {
	fsm.writeOutBoundLock.Lock()
	defer fsm.writeOutBoundLock.Unlock()

	filePathName := filepath.Join(fsm.folder, "outboundtx.dat")
	buf, err := json.Marshal(reqs)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to marshal the outbound tx")
		return err
	}
	return ioutil.WriteFile(filePathName, buf, 0o600)
}

func (fsm *TxStateMgr) LoadOutBoundState() ([]*common.OutBoundReq, error) {
	if len(fsm.folder) < 1 {
		return nil, errors.New("base file path is invalid")
	}
	filePathName := filepath.Join(fsm.folder, "outboundtx.dat")
	_, err := os.Stat(filePathName)
	if err != nil {
		return nil, err
	}
	fsm.writeOutBoundLock.RLock()
	input, err := ioutil.ReadFile(filePathName)
	if err != nil {
		fsm.writeOutBoundLock.RUnlock()
		return nil, err
	}
	fsm.writeOutBoundLock.RUnlock()
	var outboundReq []*common.OutBoundReq
	err = json.Unmarshal(input, &outboundReq)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to unmarshal the outbound req")
	}
	return outboundReq, err
}

func (fsm *TxStateMgr) SaveInBoundState(reqs []*common.InBoundReq) error {
	fsm.writeInBoundLock.Lock()
	defer fsm.writeInBoundLock.Unlock()

	filePathName := filepath.Join(fsm.folder, "inboundtx.dat")
	buf, err := json.Marshal(reqs)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to marshal the outbound tx")
		return err
	}
	return ioutil.WriteFile(filePathName, buf, 0o600)
}

func (fsm *TxStateMgr) LoadInBoundState() ([]*common.InBoundReq, error) {
	if len(fsm.folder) < 1 {
		return nil, errors.New("base file path is invalid")
	}
	filePathName := filepath.Join(fsm.folder, "inboundtx.dat")
	_, err := os.Stat(filePathName)
	if err != nil {
		return nil, err
	}
	fsm.writeOutBoundLock.RLock()
	input, err := ioutil.ReadFile(filePathName)
	if err != nil {
		fsm.writeOutBoundLock.RUnlock()
		return nil, err
	}
	fsm.writeOutBoundLock.RUnlock()
	var inboundReq []*common.InBoundReq
	err = json.Unmarshal(input, &inboundReq)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to unmarshal the outbound req")
	}
	return inboundReq, err
}

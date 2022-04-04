package storage

import (
	"encoding/json"
	"errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/joltify/joltifychain-bridge/common"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// LocalStateManager provide necessary methods to manage the local state, save it , and read it back
// LocalStateManager doesn't have any opinion in regards to where it should be persistent to
type LocalStateManager interface {
	SaveOutBoundState([]*common.OutBoundReq) error
	LoadOutBoundState() []*common.OutBoundReq
}

// TxStateMgr save the local state to file
type TxStateMgr struct {
	folder    string
	writeLock *sync.RWMutex
	logger    zerolog.Logger
}

// NewTxStateMgr create a new instance of the FileStateMgr which implements LocalStateManager
func NewTxStateMgr(folder string) *TxStateMgr {
	logger := log.With().Str("module", "pubchain").Logger()
	return &TxStateMgr{
		folder:    folder,
		writeLock: &sync.RWMutex{},
		logger:    logger,
	}
}

func (fsm *TxStateMgr) SaveOutBoundState(reqs []*common.OutBoundReq) error {
	fsm.writeLock.Lock()
	defer fsm.writeLock.Unlock()

	filePathName := filepath.Join(fsm.folder, "outboundtx.dat")
	_, err := os.Stat(filePathName)
	if err != nil {
		buf, err := json.Marshal(reqs)
		if err != nil {
			fsm.logger.Error().Err(err).Msgf("fail to marshal the outbound tx")
			return err
		}
		return ioutil.WriteFile(filePathName, buf, 0o655)
	}

	input, err := ioutil.ReadFile(filePathName)
	if err != nil {
		return err
	}

	var outboundReq []*common.OutBoundReq
	err = json.Unmarshal(input, &outboundReq)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to unmarshal the outbound req")
	}

	outboundReq = append(outboundReq, reqs...)

	buf, err := json.Marshal(outboundReq)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to marshal the outbound tx")
		return err
	}
	return ioutil.WriteFile(filePathName, buf, 0o655)
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
	fsm.writeLock.RLock()
	input, err := ioutil.ReadFile(filePathName)
	if err != nil {
		fsm.writeLock.RUnlock()
		return nil, err
	}
	fsm.writeLock.RUnlock()
	var outboundReq []*common.OutBoundReq
	err = json.Unmarshal(input, &outboundReq)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to unmarshal the outbound req")
	}
	return outboundReq, err
}

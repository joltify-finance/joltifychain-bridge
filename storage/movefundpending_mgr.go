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
	bcommon "gitlab.com/oppy-finance/oppy-bridge/common"
)

// PendingMoveFundMgr save the local state to file
type PendingMoveFundMgr struct {
	folder           string
	writePendingLock *sync.RWMutex
	logger           zerolog.Logger
}

// NewPendingTxStateMgr create a new instance of the FileStateMgr which implements LocalStateManager
func NewMoveFundStateMgr(folder string) *PendingMoveFundMgr {
	logger := log.With().Str("module", "movefundsave").Logger()
	return &PendingMoveFundMgr{
		folder:           folder,
		writePendingLock: &sync.RWMutex{},
		logger:           logger,
	}
}

func (fsm *PendingMoveFundMgr) SavePendingItems(pendingTxsPub, pendingTxOppy []*bcommon.PoolInfo) error {
	fsm.writePendingLock.Lock()
	defer fsm.writePendingLock.Unlock()

	pubfile := filepath.Join(fsm.folder, "movefundpending_pub.dat")
	oppyfile := filepath.Join(fsm.folder, "movefundpending_oppy.dat")
	bufPub, err := json.Marshal(pendingTxsPub)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to marshal the inbound pending tx")
		return err
	}

	bufOppy, err := json.Marshal(pendingTxOppy)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to marshal the inbound pending tx")
		return err
	}

	var errtwo error
	errtwo = nil
	err1 := ioutil.WriteFile(pubfile, bufPub, 0600)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to load the pub move fund")
		errtwo = err1
	}
	err2 := ioutil.WriteFile(oppyfile, bufOppy, 0600)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to load the oppy move fund")
		errtwo = err2
	}
	return errtwo
}

func (fsm *PendingMoveFundMgr) LoadPendingItems() ([]*bcommon.PoolInfo, []*bcommon.PoolInfo, error) {
	var moveFundPendingPub, moveFundPendingOppy []*bcommon.PoolInfo
	if len(fsm.folder) < 1 {
		return nil, nil, errors.New("base file path is invalid")
	}
	pubFilePathName := filepath.Join(fsm.folder, "movefundpending_pub.dat")
	_, err := os.Stat(pubFilePathName)
	if err != nil {
		fsm.logger.Error().Err(err).Msgf("fail to load the move fund on pub chain")
		pubFilePathName = ""
	}

	oppyFilePathName := filepath.Join(fsm.folder, "movefundpending_oppy.dat")
	_, err = os.Stat(oppyFilePathName)
	if err != nil {
		oppyFilePathName = ""
		fsm.logger.Error().Err(err).Msgf("fail to load the move fund on oppy chain")
	}

	fsm.writePendingLock.RLock()
	defer fsm.writePendingLock.RUnlock()
	if len(pubFilePathName) != 0 {
		inputPub, err := ioutil.ReadFile(pubFilePathName)
		if err != nil {
			fsm.logger.Error().Err(err).Msgf("fail to read the move fund on pub chain")
		}

		err = json.Unmarshal(inputPub, &moveFundPendingPub)
		if err != nil {
			fsm.logger.Error().Err(err).Msgf("fail to unmarshal the inbound pending tx")
		}
	}

	if len(oppyFilePathName) != 0 {
		inputOppy, err := ioutil.ReadFile(oppyFilePathName)
		if err != nil {
			fsm.logger.Error().Err(err).Msgf("fail to read the move fund on pub chain")
		}
		err = json.Unmarshal(inputOppy, &moveFundPendingOppy)
		if err != nil {
			fsm.logger.Error().Err(err).Msgf("fail to unmarshal the inbound pending tx")
		}
	}
	return moveFundPendingPub, moveFundPendingOppy, nil
}

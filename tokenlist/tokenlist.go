package tokenlist

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type TokenList struct {
	joltTokenList *sync.Map
	pubTokenList  *sync.Map
	updateGap     int64
	filePath      string
	logger        zerolog.Logger
}

// NewTxStateMgr create a new instance of the FileStateMgr which implements LocalStateManager
func NewTokenList(filePath string, updateGap int64) (*TokenList, error) {
	logger := log.With().Str("module", "tokenlist").Logger()
	// process tokenlist.json file
	dat, err := ioutil.ReadFile(filePath)
	if err != nil {
		logger.Error().Err(err).Msgf("error in read with file %v", filePath)
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(dat, &result)
	if err != nil {
		logger.Error().Err(err).Msgf("fail to unmarshal the tokenlist.json")
		return nil, errors.New("fail to process the tokenlist.json")
	}
	// if the tokenlist.json is empty, fail to create bridge service
	if len(result) == 0 {
		logger.Error().Err(err).Msgf("%v is empty", filePath)
		return nil, errors.New("tokenlist.json is empty")
	}

	// init TokenList
	tl := &TokenList{
		joltTokenList: &sync.Map{},
		pubTokenList:  &sync.Map{},
		updateGap:     updateGap,
		filePath:      filePath,
		logger:        logger,
	}

	// load token list
	for tokenAddr, tokenDenom := range result {
		tl.pubTokenList.Store(tokenAddr, tokenDenom.(string))
		tl.joltTokenList.Store(tokenDenom.(string), tokenAddr)
	}
	tl.logger.Info().Msgf("token list is created from %v", tl.filePath)
	return tl, nil
}

func (tl *TokenList) UpdateTokenList(currentBlockHeight int64) error {
	// check if the tokenlist needs to be updated
	if currentBlockHeight%tl.updateGap != 0 {
		return nil
	}

	// load and process the tokenlist.json file
	dat, err := ioutil.ReadFile(tl.filePath)
	if err != nil {
		tl.logger.Error().Err(err).Msgf("error in read token list file")
		return err
	}
	var result map[string]interface{}
	err = json.Unmarshal(dat, &result)
	if err != nil {
		tl.logger.Error().Err(err).Msgf("fail to unmarshal the tokenlist_history.json")
		return errors.New("fail to process the tokenlist.json")
	}
	// if the tokenlist.json is empty, fail to create bridge service
	if len(result) == 0 {
		tl.logger.Error().Err(err).Msgf("%v is an empty", tl.filePath)
		return errors.New("tokenlist.json is empty")
	}

	// create a new token list
	newJoltTokenlist := &sync.Map{}
	newPubTokenlist := &sync.Map{}
	for tokenAddr, tokenDenom := range result {
		newPubTokenlist.Store(tokenAddr, tokenDenom.(string))
		newJoltTokenlist.Store(tokenDenom.(string), tokenAddr)
	}

	// update the token list
	tl.joltTokenList = newJoltTokenlist
	tl.pubTokenList = newPubTokenlist
	tl.logger.Info().Msgf("Token List is updated")
	return nil
}

func (tl *TokenList) GetTokenDenom(tokenAddr string) (string, bool) {
	tokenDenom, exist := tl.pubTokenList.Load(tokenAddr)
	tokenDenomStr, _ := tokenDenom.(string)
	return tokenDenomStr, exist
}

func (tl *TokenList) GetTokenAddress(tokenDenom string) (string, bool) {
	tokenAddr, exist := tl.joltTokenList.Load(tokenDenom)
	tokenAddrStr, _ := tokenAddr.(string)
	return tokenAddrStr, exist
}

func (tl *TokenList) GetAllExistedTokenAddresses() []string {
	tokenInfo := []string{}
	tl.pubTokenList.Range(func(tokenAddr, tokenDenom interface{}) bool {
		tokenAddrStr, _ := tokenAddr.(string)
		tokenInfo = append(tokenInfo, tokenAddrStr)
		return true
	})
	return tokenInfo
}

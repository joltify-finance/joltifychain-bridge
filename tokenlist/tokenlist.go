package tokenlist

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type TokenItem struct {
	TokenAddr string `json:"token_addr"`
	Denom     string `json:"denom"`
	Decimals  int    `json:"decimals"`
}

type TokenList struct {
	oppyTokenList *sync.Map
	pubTokenList  *sync.Map
	updateGap     int64
	filePath      string
	logger        zerolog.Logger
}

type BridgeTokenListI interface {
	GetTokenInfoByDenom(tokenDenom string) (TokenItem, bool)
	GetTokenInfoByAddress(tokenAddress string) (TokenItem, bool)
	GetAllExistedTokenAddresses() []string
}

// NewTokenList creates a new instance of the FileStateMgr which implements LocalStateManager
func NewTokenList(filePath string, updateGap int64) (*TokenList, error) {
	logger := log.With().Str("module", "tokenlist").Logger()
	// process tokenlist.json file
	dat, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error().Err(err).Msgf("error in read with file %v", filePath)
		return nil, err
	}
	var tokensItems []TokenItem
	err = json.Unmarshal(dat, &tokensItems)
	if err != nil {
		logger.Error().Err(err).Msgf("fail to unmarshal the tokenlist.json")
		return nil, errors.New("fail to process the tokenlist.json")
	}
	// if the tokenlist.json is empty, fail to create bridge service
	if len(tokensItems) == 0 {
		logger.Error().Err(err).Msgf("%v is empty", filePath)
		return nil, errors.New("tokenlist.json is empty")
	}

	// init TokenList
	tl := &TokenList{
		oppyTokenList: &sync.Map{},
		pubTokenList:  &sync.Map{},
		updateGap:     updateGap,
		filePath:      filePath,
		logger:        logger,
	}

	// load token list
	for _, item := range tokensItems {
		tl.pubTokenList.Store(strings.ToLower(item.TokenAddr), item)
		tl.oppyTokenList.Store(strings.ToLower(item.Denom), item)
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
	dat, err := os.ReadFile(tl.filePath)
	if err != nil {
		tl.logger.Error().Err(err).Msgf("error in read token list file")
		return err
	}

	var tokensItems []TokenItem
	err = json.Unmarshal(dat, &tokensItems)
	if err != nil {
		tl.logger.Error().Err(err).Msgf("fail to unmarshal the tokenlist_history.json")
		return errors.New("fail to process the tokenlist.json")
	}
	// if the tokenlist.json is empty, fail to create bridge service
	if len(tokensItems) == 0 {
		tl.logger.Error().Err(err).Msgf("%v is an empty", tl.filePath)
		return errors.New("tokenlist.json is empty")
	}

	// create a new token list
	newOppyTokenlist := &sync.Map{}
	newPubTokenlist := &sync.Map{}

	for _, item := range tokensItems {
		newPubTokenlist.Store(strings.ToLower(item.TokenAddr), item)
		newOppyTokenlist.Store(strings.ToLower(item.Denom), item)
	}

	// update the token list
	tl.oppyTokenList = newOppyTokenlist
	tl.pubTokenList = newPubTokenlist
	tl.logger.Info().Msgf("Token List is updated")
	return nil
}

func (tl *TokenList) GetTokenInfoByDenom(denom string) (TokenItem, bool) {
	tokenItem, exist := tl.oppyTokenList.Load(strings.ToLower(denom))
	item, _ := tokenItem.(TokenItem)
	return item, exist
}

func (tl *TokenList) GetTokenInfoByAddress(addr string) (TokenItem, bool) {
	tokenItem, exist := tl.pubTokenList.Load(strings.ToLower(addr))
	item, _ := tokenItem.(TokenItem)
	return item, exist
}

func (tl *TokenList) GetAllExistedTokenAddresses() []string {
	var tokenAddresses []string
	tl.pubTokenList.Range(func(tokenAddr, _ interface{}) bool {
		item, _ := tokenAddr.(string)
		tokenAddresses = append(tokenAddresses, strings.ToLower(item))
		return true
	})
	return tokenAddresses
}

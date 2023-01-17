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
	ChainType string `json:"chain_type"`
}

type TokenList struct {
	oppyTokenList *sync.Map
	pubTokenList  *sync.Map
	updateGap     int64
	filePath      string
	logger        zerolog.Logger
}

type BridgeTokenListI interface {
	GetTokenInfoByDenomAndChainType(tokenDenom, chainType string) (TokenItem, bool)
	GetTokenInfoByAddressAndChainType(tokenAddress, chainType string) (TokenItem, bool)
	GetAllExistedTokenAddresses(chainType string) []string
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
		tl.pubTokenList.Store(strings.ToLower(item.TokenAddr+":"+item.ChainType), item)
		tl.oppyTokenList.Store(strings.ToLower(item.Denom+":"+item.ChainType), item)
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
		newPubTokenlist.Store(strings.ToLower(item.TokenAddr+":"+item.ChainType), item)
		newOppyTokenlist.Store(strings.ToLower(item.Denom+":"+item.ChainType), item)
	}

	// update the token list
	tl.oppyTokenList = newOppyTokenlist
	tl.pubTokenList = newPubTokenlist
	tl.logger.Info().Msgf("Token List is updated")
	return nil
}

func (tl *TokenList) GetTokenInfoByDenomAndChainType(denom, chainType string) (TokenItem, bool) {
	tokenItem, exist := tl.oppyTokenList.Load(strings.ToLower(denom + ":" + chainType))
	item, _ := tokenItem.(TokenItem)
	return item, exist
}

func (tl *TokenList) GetTokenInfoByAddressAndChainType(addr, chainType string) (TokenItem, bool) {
	tokenItem, exist := tl.pubTokenList.Load(strings.ToLower(addr + ":" + chainType))
	item, _ := tokenItem.(TokenItem)
	return item, exist
}

func (tl *TokenList) GetAllExistedTokenAddresses(chainType string) []string {
	var tokenAddresses []string
	tl.pubTokenList.Range(func(tokenAddrWithType, _ interface{}) bool {
		it, _ := tokenAddrWithType.(string)
		items := strings.Split(it, ":")
		if items[1] == strings.ToLower(chainType) {
			tokenAddresses = append(tokenAddresses, strings.ToLower(items[0]))
		}
		return true
	})
	return tokenAddresses
}

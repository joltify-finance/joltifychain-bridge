package tokenlist

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// type TokenInfo struct {
// 	Denom         string
// 	TokenInstance *generated.Token
// }

type TokenList struct {
	JoltTokenList    *sync.Map
	PubTokenList     *sync.Map
	HistoryTokenList map[string]string
	UpdateMark       int64
	UpdateGap        int64
	FolderPath       string
	// PubClient        bind.ContractBackend
	logger zerolog.Logger
}

// NewTxStateMgr create a new instance of the FileStateMgr which implements LocalStateManager
func NewTokenList(folderPath string, updateGap int64) (*TokenList, error) {
	logger := log.With().Str("module", "tokenlist").Logger()
	TL := &TokenList{
		FolderPath: folderPath,
		UpdateMark: 0,
		UpdateGap:  updateGap,
		// PubClient:  ethClient,
		logger: logger,
	}

	// process tokenlist.json file
	tokenlist_path := filepath.Join(folderPath, "tokenlist.json")
	dat, err := ioutil.ReadFile(tokenlist_path)
	if err != nil {
		TL.logger.Error().Err(err).Msgf("error in read tokenlist.json file")
		return TL, err
	}
	var result map[string]interface{}
	json.Unmarshal([]byte(dat), &result)
	// if the tokenlist.json is empty, fail to create bridge service
	if len(result) == 0 {
		TL.logger.Error().Err(err).Msgf("%v is an empty", tokenlist_path)
		return TL, err
	}

	// load token list
	jolt_tokenlist := new(sync.Map)
	pub_tokenlist := new(sync.Map)
	hist_tokenlist := make(map[string]string)
	for tokenAddr, tokenDenom := range result {
		// tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), ethClient)
		// if err != nil {
		// 	TL.logger.Error().Err(err).Msgf("fail to process tokenlist.json file")
		// 	return TL, errors.New("fail to process tokenlist.json file")
		// }
		// tokenInfo := TokenInfo{
		// 	Denom:         tokenDenom.(string),
		// 	TokenInstance: tokenInstance,
		// }
		pub_tokenlist.Store(tokenAddr, tokenDenom.(string))
		jolt_tokenlist.Store(tokenDenom.(string), tokenAddr)
		// update the current tokenlist.json to tokenlist_history.json too
		hist_tokenlist[tokenAddr] = tokenDenom.(string)
	}
	TL.JoltTokenList = jolt_tokenlist
	TL.PubTokenList = pub_tokenlist

	// load history token list
	tokenlist_history_path := filepath.Join(folderPath, "tokenlist_history.json")
	hist_dat, err := ioutil.ReadFile(tokenlist_history_path)
	if err != nil {
		TL.logger.Error().Err(err).Msgf("error in read tokenlist_history.json file")
		return TL, err
	}
	var hist_result map[string]interface{}
	json.Unmarshal([]byte(hist_dat), &hist_result)
	// if the tokenlist_history.json is empty, fail to create bridge service
	if len(hist_result) == 0 {
		TL.logger.Error().Err(err).Msgf("%v is an empty", tokenlist_history_path)
		return TL, err
	}
	for tokenAddr, tokenDenom := range hist_result {
		_, ok := hist_tokenlist[tokenAddr]
		if !ok {
			hist_tokenlist[tokenAddr] = tokenDenom.(string)
		}
	}
	TL.HistoryTokenList = hist_tokenlist
	return TL, nil
}

func (tl *TokenList) UpdateTokenList(currentBlockHeight int64) error {
	// check if the tokenlist needs to be updated
	if currentBlockHeight/tl.UpdateGap <= tl.UpdateMark {
		return nil
	}
	updateMark := currentBlockHeight / tl.UpdateGap

	// load and process the tokenlist.json file
	tokenlist_path := filepath.Join(tl.FolderPath, "tokenlist.json")
	dat, err := ioutil.ReadFile(tokenlist_path)
	if err != nil {
		tl.logger.Error().Err(err).Msgf("error in read token list file")
		return err
	}
	var result map[string]interface{}
	json.Unmarshal([]byte(dat), &result)
	// if the tokenlist.json is empty, fail to create bridge service
	if len(result) == 0 {
		tl.logger.Error().Err(err).Msgf("%v is an empty", tokenlist_path)
		return err
	}

	// create a new token list
	new_jolt_tokenlist := new(sync.Map)
	new_pub_tokenlist := new(sync.Map)
	for tokenAddr, tokenDenom := range result {
		// tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), tl.PubClient)
		// if err != nil {
		// 	return err
		// }
		// tokenInfo := TokenInfo{
		// 	Denom:         tokenDenom.(string),
		// 	TokenInstance: tokenInstance,
		// }
		new_pub_tokenlist.Store(tokenAddr, tokenDenom.(string))
		new_jolt_tokenlist.Store(tokenDenom.(string), tokenAddr)
		// update history token list
		_, ok := tl.HistoryTokenList[tokenAddr]
		if !ok {
			tl.HistoryTokenList[tokenAddr] = tokenDenom.(string)
		}
	}

	// update the token list
	tl.JoltTokenList = new_jolt_tokenlist
	tl.PubTokenList = new_pub_tokenlist
	tl.UpdateMark = updateMark
	return nil
}

func (tl *TokenList) ExportHistoryTokenList() error {
	filePathName := filepath.Join(tl.FolderPath, "tokenlist_history.json")
	buf, err := json.Marshal(&(tl.HistoryTokenList))
	if err != nil {
		tl.logger.Error().Err(err).Msgf("fail to marshal the history token list")
		return err
	}
	return ioutil.WriteFile(filePathName, buf, 0o655)
}

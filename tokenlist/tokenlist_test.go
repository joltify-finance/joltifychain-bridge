package tokenlist

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func getTokenListFilePath(filapath string) (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	tokenlistPath := path.Join(current, filapath)
	return tokenlistPath, nil
}

func TestWriteToken(t *testing.T) {
	usdt := TokenItem{
		strings.ToLower("0xf2CfA2606b55352164ba86dEfa50A5E57bEC888e"),
		"ausdt",
		6,
		"BSC",
	}
	jusd := TokenItem{
		strings.ToLower("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"),
		"ajusd",
		18,
		"BSC",
	}

	jolt := TokenItem{
		strings.ToLower("0x15fb343d82cD1C22542261dF408dA8396A829F6B"),
		"ajolt",
		18,
		"BSC",
	}
	bnb := TokenItem{
		"native",
		"abnb",
		18,
		"BSC",
	}

	allItems := []TokenItem{usdt, jusd, jolt, bnb}

	out, err := json.Marshal(allItems)
	assert.NilError(t, err)
	err = ioutil.WriteFile("../test_data/tokenlist/tokenlist.json", out, 0o600)
	assert.NilError(t, err)
}

func TestNewTokenList(t *testing.T) {
	var tokenlistPath string

	tokenlistPath, err := getTokenListFilePath("../nonExistedPath")
	assert.NilError(t, err)
	_, err = NewTokenList(tokenlistPath, 100)
	assert.ErrorContains(t, err, "no such file or directory")

	tokenlistPath, err = getTokenListFilePath("../test_data/tokenlist/tokenlist_empty.json")
	assert.NilError(t, err)
	_, err = NewTokenList(tokenlistPath, 100)
	assert.Error(t, err, "tokenlist.json is empty")

	tokenlistPath, err = getTokenListFilePath("../test_data/tokenlist/tokenlist_bad.json")
	assert.NilError(t, err)
	_, err = NewTokenList(tokenlistPath, 100)
	assert.Error(t, err, "fail to process the tokenlist.json")

	tokenlistPath, err = getTokenListFilePath("../test_data/tokenlist/tokenlist.json")
	assert.NilError(t, err)
	tl, err := NewTokenList(tokenlistPath, 100)
	assert.NilError(t, err)
	assert.Equal(t, tl.updateGap, int64(100))

	// check token existence
	tokenItem, exit := tl.GetTokenInfoByDenomAndChainType("ajusd", "BSC")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenItem.TokenAddr, strings.ToLower("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"))
	tokenItem, exit = tl.GetTokenInfoByDenomAndChainType("ajolt", "BSC")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenItem.TokenAddr, strings.ToLower("0x15fb343d82cD1C22542261dF408dA8396A829F6B"))
	tokenItem, exit = tl.GetTokenInfoByDenomAndChainType("nonExistedDenom", "BSC")
	assert.Equal(t, exit, false)
	assert.Equal(t, tokenItem.TokenAddr, "")

	// check tl.PubTokenlist
	tokenItem, exit = tl.GetTokenInfoByAddressAndChainType("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79", "BSC")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenItem.Denom, strings.ToLower("aJUSD"))
	tokenItem, exit = tl.GetTokenInfoByAddressAndChainType("0x15fb343d82cD1C22542261dF408dA8396A829F6B", "BSC")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenItem.Denom, strings.ToLower("aJolt"))
	tokenItem, exit = tl.GetTokenInfoByAddressAndChainType("nonExistedAddress", "BSC")
	assert.Equal(t, exit, false)
	assert.Equal(t, tokenItem.Denom, "")
}

func TestUpdateTokenList(t *testing.T) {
	tokenlistPath, err := getTokenListFilePath("../test_data/tokenlist/tokenlist.json")
	assert.NilError(t, err)
	tl, err := NewTokenList(tokenlistPath, 100)
	assert.NilError(t, err)
	assert.Equal(t, tl.updateGap, int64(100))

	tl.filePath, err = getTokenListFilePath("../nonExistedPath")
	assert.NilError(t, err)
	err = tl.UpdateTokenList(int64(100))
	assert.ErrorContains(t, err, "no such file or directory")

	tl.filePath, err = getTokenListFilePath("../test_data/tokenlist/tokenlist_empty.json")
	assert.NilError(t, err)
	err = tl.UpdateTokenList(int64(100))
	assert.Error(t, err, "tokenlist.json is empty")

	tl.filePath, err = getTokenListFilePath("../test_data/tokenlist/tokenlist_bad.json")
	assert.NilError(t, err)
	err = tl.UpdateTokenList(int64(100))
	assert.Error(t, err, "fail to process the tokenlist.json")

	tokenlistPathUpdate, err := getTokenListFilePath("../test_data/tokenlist/tokenlist_updated.json")
	assert.NilError(t, err)
	tl.filePath = tokenlistPathUpdate
	err = tl.UpdateTokenList(int64(100))
	assert.NilError(t, err)

	_, exit := tl.GetTokenInfoByDenomAndChainType("aJUSD", "BSC")
	assert.Equal(t, exit, false)
	tokenItem, exit := tl.GetTokenInfoByDenomAndChainType("aJolt", "BSC")
	assert.Equal(t, exit, true)
	assert.Equal(t, strings.ToLower(tokenItem.TokenAddr), strings.ToLower("0x15fb343d82cD1C22542261dF408dA8396A829F6B"))
	item, exit := tl.GetTokenInfoByDenomAndChainType("testUpdateDenom", "BSC")
	assert.Equal(t, exit, true)
	assert.Equal(t, strings.ToLower(item.TokenAddr), strings.ToLower("testUpdateAddress"))

	// check tl.PubTokenlist
	_, exit = tl.pubTokenList.Load("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")
	assert.Equal(t, exit, false)
	tokenItem, exit = tl.GetTokenInfoByAddressAndChainType("0x15fb343d82cD1C22542261dF408dA8396A829F6B", "BSC")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenItem.Denom, strings.ToLower("aJolt"))
	tokenItem, exit = tl.GetTokenInfoByAddressAndChainType("testUpdateAddress", "BSC")
	assert.Equal(t, exit, true)
	assert.Equal(t, strings.ToLower(tokenItem.Denom), strings.ToLower("testUpdateDenom"))
}

func TestTokenListAccess(t *testing.T) {
	tokenlistPath, err := getTokenListFilePath("../test_data/tokenlist/tokenlist.json")
	assert.NilError(t, err)
	tl, err := NewTokenList(tokenlistPath, 100)
	assert.NilError(t, err)
	assert.Equal(t, tl.updateGap, int64(100))

	tokenItem, exist := tl.GetTokenInfoByDenomAndChainType("aJUSD", "BSC")
	assert.Equal(t, exist, true)
	assert.Equal(t, tokenItem.TokenAddr, strings.ToLower("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"))

	tokenItem, exist = tl.GetTokenInfoByDenomAndChainType("ajolt", "BSC")
	assert.Equal(t, exist, true)
	assert.Equal(t, tokenItem.TokenAddr, strings.ToLower("0x15fb343d82cD1C22542261dF408dA8396A829F6B"))

	tokenItem, exist = tl.GetTokenInfoByDenomAndChainType("nonExistedDenom", "BSC")
	assert.Equal(t, exist, false)
	assert.Equal(t, tokenItem.TokenAddr, "")

	tokenItem, exist = tl.GetTokenInfoByAddressAndChainType("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79", "BSC")
	assert.Equal(t, exist, true)
	assert.Equal(t, tokenItem.Denom, strings.ToLower("aJUSD"))

	tokenItem, exist = tl.GetTokenInfoByAddressAndChainType("0x15fb343d82cD1C22542261dF408dA8396A829F6B", "BSC")
	assert.Equal(t, exist, true)
	assert.Equal(t, tokenItem.Denom, strings.ToLower("ajolt"))

	tokenItem, exist = tl.GetTokenInfoByAddressAndChainType("nonExistedAddress", "BSC")
	assert.Equal(t, exist, false)
	assert.Equal(t, tokenItem.Denom, "")
}

func TestGetAllExistedTokenAddresses(t *testing.T) {
	tokenlistPath, err := getTokenListFilePath("../test_data/tokenlist/tokenlist.json")
	assert.NilError(t, err)
	tl, err := NewTokenList(tokenlistPath, 100)
	assert.NilError(t, err)
	tokenAddresses := tl.GetAllExistedTokenAddresses("bsc")
	assert.Equal(t, len(tokenAddresses), 4)
}

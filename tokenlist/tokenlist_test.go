package tokenlist

import (
	"os"
	"path"
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

	// check tl.JoltTokenlist
	var exit bool
	var tokenAddr string
	var tokenDenom string
	tokenAddr, exit = tl.GetTokenAddress("JUSD")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenAddr, "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")
	tokenAddr, exit = tl.GetTokenAddress("JoltBNB")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenAddr, "0x15fb343d82cD1C22542261dF408dA8396A829F6B")
	tokenAddr, exit = tl.GetTokenAddress("nonExistedDenom")
	assert.Equal(t, exit, false)
	assert.Equal(t, tokenAddr, "")

	// check tl.PubTokenlist
	tokenDenom, exit = tl.GetTokenDenom("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JUSD")
	tokenDenom, exit = tl.GetTokenDenom("0x15fb343d82cD1C22542261dF408dA8396A829F6B")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JoltBNB")
	tokenDenom, exit = tl.GetTokenDenom("nonExistedAddress")
	assert.Equal(t, exit, false)
	assert.Equal(t, tokenDenom, "")
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

	var exit bool
	var tokenAddr string
	var tokenDenom string

	tokenAddr, exit = tl.GetTokenAddress("JUSD")
	tl.logger.Info().Msgf("the vaule: %v", tokenAddr)
	assert.Equal(t, exit, false)
	tokenAddr, exit = tl.GetTokenAddress("JoltBNB")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenAddr, "0x15fb343d82cD1C22542261dF408dA8396A829F6B")
	tokenAddr, exit = tl.GetTokenAddress("testUpdateDenom")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenAddr, "testUpdateAddress")

	// check tl.PubTokenlist
	_, exit = tl.pubTokenList.Load("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")
	assert.Equal(t, exit, false)
	tokenDenom, exit = tl.GetTokenDenom("0x15fb343d82cD1C22542261dF408dA8396A829F6B")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JoltBNB")
	tokenDenom, exit = tl.GetTokenDenom("testUpdateAddress")
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "testUpdateDenom")
}

func TestTokenListAccess(t *testing.T) {
	tokenlistPath, err := getTokenListFilePath("../test_data/tokenlist/tokenlist.json")
	assert.NilError(t, err)
	tl, err := NewTokenList(tokenlistPath, 100)
	assert.NilError(t, err)
	assert.Equal(t, tl.updateGap, int64(100))

	tokenAddr, exist := tl.GetTokenAddress("JUSD")
	assert.Equal(t, exist, true)
	assert.Equal(t, tokenAddr, "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")

	tokenAddr, exist = tl.GetTokenAddress("JoltBNB")
	assert.Equal(t, exist, true)
	assert.Equal(t, tokenAddr, "0x15fb343d82cD1C22542261dF408dA8396A829F6B")

	tokenAddr, exist = tl.GetTokenAddress("nonExistedDenom")
	assert.Equal(t, exist, false)
	assert.Equal(t, tokenAddr, "")

	tokenDenom, exist := tl.GetTokenDenom("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")
	assert.Equal(t, exist, true)
	assert.Equal(t, tokenDenom, "JUSD")

	tokenDenom, exist = tl.GetTokenDenom("0x15fb343d82cD1C22542261dF408dA8396A829F6B")
	assert.Equal(t, exist, true)
	assert.Equal(t, tokenDenom, "JoltBNB")

	tokenDenom, exist = tl.GetTokenDenom("nonExistedAddress")
	assert.Equal(t, exist, false)
	assert.Equal(t, tokenDenom, "")
}

func TestGetAllExistedTokenAddresses(t *testing.T) {
	tokenlistPath, err := getTokenListFilePath("../test_data/tokenlist/tokenlist.json")
	assert.NilError(t, err)
	tl, err := NewTokenList(tokenlistPath, 100)
	assert.NilError(t, err)
	tokenAddresses := tl.GetAllExistedTokenAddresses()
	count := 0
	for _, tokenAddr := range tokenAddresses {
		if tokenAddr == "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79" {
			count += 1
		}
		if tokenAddr == "0x15fb343d82cD1C22542261dF408dA8396A829F6B" {
			count += 1
		}
	}
	assert.Equal(t, count, 2)
}

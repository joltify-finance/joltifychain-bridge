package tokenlist

import (
	"strings"
	"sync"
)

type MockTokenList struct {
	joltTokenList *sync.Map
	pubTokenList  *sync.Map
}

func (mt *MockTokenList) GetTokenInfoByAddressAndChainType(tokenAddr, chainType string) (TokenItem, bool) {
	mt.pubTokenList.Range(func(key, value any) bool {
		return true
	})

	tokenDenom, exist := mt.pubTokenList.Load(strings.ToLower(tokenAddr + ":" + chainType))
	tokenDenomStr, _ := tokenDenom.(string)
	return TokenItem{
		TokenAddr: strings.ToLower(tokenAddr),
		Denom:     strings.ToLower(tokenDenomStr),
		Decimals:  18,
		ChainType: chainType,
	}, exist
}

func (mt *MockTokenList) GetTokenInfoByDenomAndChainType(tokenDenom, chainType string) (TokenItem, bool) {
	tokenAddr, exist := mt.joltTokenList.Load(strings.ToLower(tokenDenom + ":" + chainType))
	tokenAddrStr, _ := tokenAddr.(string)
	return TokenItem{
		TokenAddr: strings.ToLower(tokenAddrStr),
		Denom:     strings.ToLower(tokenDenom),
		Decimals:  18,
		ChainType: chainType,
	}, exist
}

func (mt *MockTokenList) GetAllExistedTokenAddresses(chainType string) []string {
	tokenInfo := []string{}
	mt.pubTokenList.Range(func(tokenAddr, _ interface{}) bool {
		it, _ := tokenAddr.(string)
		items := strings.Split(it, ":")
		if items[1] == strings.ToLower(chainType) {
			tokenInfo = append(tokenInfo, strings.ToLower(items[0]))
		}
		return true
	})
	return tokenInfo
}

func CreateMockTokenlist(tokenAddr []string, tokenDenom []string, chainType []string) (*MockTokenList, error) {
	mTokenList := MockTokenList{&sync.Map{}, &sync.Map{}}
	for i, el := range tokenAddr {
		mTokenList.joltTokenList.Store(strings.ToLower(tokenDenom[i]+":"+chainType[i]), strings.ToLower(el))
		mTokenList.pubTokenList.Store(strings.ToLower(el+":"+chainType[i]), strings.ToLower(tokenDenom[i]))
	}
	return &mTokenList, nil
}

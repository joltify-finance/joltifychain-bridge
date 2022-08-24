package tokenlist

import (
	"strings"
	"sync"
)

type MockTokenList struct {
	oppyTokenList *sync.Map
	pubTokenList  *sync.Map
}

func (mt *MockTokenList) GetTokenInfoByAddress(tokenAddr string) (TokenItem, bool) {
	tokenDenom, exist := mt.pubTokenList.Load(strings.ToLower(tokenAddr))
	tokenDenomStr, _ := tokenDenom.(string)
	return TokenItem{
		TokenAddr: strings.ToLower(tokenAddr),
		Denom:     strings.ToLower(tokenDenomStr),
		Decimals:  18,
	}, exist
}

func (mt *MockTokenList) GetTokenInfoByDenom(tokenDenom string) (TokenItem, bool) {
	tokenAddr, exist := mt.oppyTokenList.Load(strings.ToLower(tokenDenom))
	tokenAddrStr, _ := tokenAddr.(string)
	return TokenItem{
		TokenAddr: strings.ToLower(tokenAddrStr),
		Denom:     strings.ToLower(tokenDenom),
		Decimals:  18,
	}, exist
}

func (mt *MockTokenList) GetAllExistedTokenAddresses() []string {
	tokenInfo := []string{}
	mt.pubTokenList.Range(func(tokenAddr, tokenDenom interface{}) bool {
		tokenAddrStr, _ := tokenAddr.(string)
		tokenInfo = append(tokenInfo, strings.ToLower(tokenAddrStr))
		return true
	})
	return tokenInfo
}

func CreateMockTokenlist(tokenAddr []string, tokenDenom []string) (*MockTokenList, error) {
	mTokenList := MockTokenList{&sync.Map{}, &sync.Map{}}
	for i, el := range tokenAddr {
		mTokenList.oppyTokenList.Store(strings.ToLower(tokenDenom[i]), strings.ToLower(el))
		mTokenList.pubTokenList.Store(strings.ToLower(el), strings.ToLower(tokenDenom[i]))
	}
	return &mTokenList, nil
}

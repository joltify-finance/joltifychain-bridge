package common

import (
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"gitlab.com/joltify/joltifychain-bridge/generated"
	// "gitlab.com/joltify/joltifychain-bridge/pubchain"
)

type TokenInfo struct {
	Denom         string
	TokenInstance *generated.Token
}

func getTokenAddresses() []string {
	return []string{"0xeB42ff4cA651c91EB248f8923358b6144c6B4b79", "0x15fb343d82cD1C22542261dF408dA8396A829F6B"}
}

func getTokenDenom() []string {
	return []string{"JUSD", "JoltBNB"}
}

func GetJoltTokenList() (*sync.Map, error) {
	tokenList := new(sync.Map)
	denomList := getTokenDenom()
	addrList := getTokenAddresses()
	if len(denomList) != len(addrList) {
		return tokenList, errors.New("The length of denomList and addrList is not equal")
	}
	for i := 0; i < len(denomList); i++ {
		tokenList.Store(denomList[i], addrList[i])
	}
	return tokenList, nil
}

func GetPubTokenList(backend bind.ContractBackend) (*sync.Map, error) {
	tokenList := new(sync.Map)
	denomList := getTokenDenom()
	tokenAddresses := getTokenAddresses()
	for i := 0; i < len(tokenAddresses); i++ {
		tokenAddr := tokenAddresses[i]
		tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), backend)
		if err != nil {
			return tokenList, errors.New("fail to get the new token")
		}
		tokenInfo := TokenInfo{
			Denom:         denomList[i],
			TokenInstance: tokenInstance,
		}
		tokenList.Store(tokenAddr, tokenInfo)
	}
	return tokenList, nil
	// for _, tokenAddr := range tokenAddresses {
	// 	tokenInstance, err := generated.NewToken(common.HexToAddress(tokenAddr), backend)
	// 	if err != nil {
	// 		return tokenList, errors.New("fail to get the new token")
	// 	}
	// 	name, err := tokenInstance.Name(&bind.CallOpts{})
	// 	if err != nil {
	// 		return tokenList, errors.New("fail to get name of the new token")
	// 	}
	// 	tokenInfo := TokenInfo{
	// 		Denom:         name,
	// 		TokenInstance: tokenInstance,
	// 	}
	// 	tokenList.Store(tokenAddr, tokenInfo)
	// }
	// return tokenList, nil
}

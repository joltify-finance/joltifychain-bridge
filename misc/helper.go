package misc

import (
	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
)

//SetupBech32Prefix sets up the prefix of the joltify chain
func SetupBech32Prefix() {
	config := types.GetConfig()
	config.SetBech32PrefixForAccount("jolt", "joltpub")
	config.SetBech32PrefixForValidator("joltval", "joltvpub")
	config.SetBech32PrefixForConsensusNode("joltvalcons", "joltcpub")
}

//PoolPubKeyToEthAddress export the joltify pubkey to the ETH format address
func PoolPubKeyToEthAddress(pk string) (common.Address, error) {
	pubkey, err := types.GetPubKeyFromBech32(sdk.Bech32PubKeyTypeAccPub, pk)
	if err != nil {
		return common.Address{}, err
	}
	addr := common.BytesToAddress(pubkey.Address().Bytes())
	return addr, nil
}

//EthAddressToJoltAddr export the joltify pubkey to the ETH format address
func EthAddressToJoltAddr(addr common.Address) (sdk.AccAddress, error) {
	jAddr, err := types.AccAddressFromHex(addr.Hex()[2:])
	return jAddr, err
}

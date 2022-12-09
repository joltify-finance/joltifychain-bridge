package common

import (
	"encoding/hex"
	"math/big"
	"strconv"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func (o *OutBoundReq) Hash() common.Hash {
	data, err := hex.DecodeString(o.TxID)
	if err != nil {
		panic(err)
	}
	needMintStr := "true"
	if o.ChainType == "OPPy" {
		needMintStr = "false"
	}
	hash := crypto.Keccak256Hash(o.OutReceiverAddress.Bytes(), []byte(o.ChainType), []byte(needMintStr), data)
	return hash
}

func NewOutboundReq(txID string, address, fromPoolAddr common.Address, coin types.Coin, coinAddr string, txBlockHeight int64, feeCoins, feeToValidator types.Coins, chainType string) OutBoundReq {
	return OutBoundReq{
		txID,
		address,
		fromPoolAddr,
		coin,
		coinAddr,
		txBlockHeight,
		uint64(0),
		common.Hash{}.Hex(),
		feeCoins,
		feeToValidator,
		chainType,
	}
}

// Index generate the index of a given inbound req
func (o *OutBoundReq) Index() string {
	data, err := hex.DecodeString(o.TxID)
	if err != nil {
		panic(err)
	}
	hash := crypto.Keccak256Hash(o.OutReceiverAddress.Bytes(), data)
	lower := hash.Big().String()
	higher := strconv.FormatInt(o.BlockHeight, 10)
	indexStr := higher + lower
	return indexStr
}

// SetItemNonce sets the block height of the tx
func (o *OutBoundReq) SetItemNonce(fromPoolAddr common.Address, nonce uint64) {
	o.FromPoolAddr = fromPoolAddr
	o.Nonce = nonce
}

// GetOutBoundInfo return the outbound tx info
func (o *OutBoundReq) GetOutBoundInfo() (common.Address, common.Address, string, *big.Int, uint64) {
	return o.OutReceiverAddress, o.FromPoolAddr, o.TokenAddr, o.Coin.Amount.BigInt(), o.Nonce
}

func (i *InBoundReq) Hash() common.Hash {
	hash := crypto.Keccak256Hash(i.UserReceiverAddress.Bytes(), i.TxID)
	return hash
}

// Index generate the index of a given inbound req
func (i *InBoundReq) Index() string {
	hash := crypto.Keccak256Hash(i.UserReceiverAddress.Bytes(), i.TxID)
	lower := hash.Big().String()
	higher := strconv.FormatInt(i.BlockHeight, 10)
	indexStr := higher + lower
	return indexStr
}

func NewAccountInboundReq(address types.AccAddress, toPoolAddr common.Address, coin types.Coin, txid []byte, blockHeight int64) InBoundReq {
	return InBoundReq{
		address,
		txid,
		toPoolAddr,
		coin,
		blockHeight,
		0,
		0,
		nil,
		"",
	}
}

// GetInboundReqInfo returns the info of the inbound transaction
func (i *InBoundReq) GetInboundReqInfo() (types.AccAddress, common.Address, types.Coin, int64) {
	return i.UserReceiverAddress, i.ToPoolAddr, i.Coin, i.BlockHeight
}

// SetAccountInfo sets the block height of the tx
func (i *InBoundReq) SetAccountInfo(number, seq uint64, address types.AccAddress, pk string) {
	i.AccNum = number
	i.AccSeq = seq
	i.PoolCosAddress = address
	i.PoolPk = pk
}

// GetAccountInfo returns the account number and seq
func (i *InBoundReq) GetAccountInfo() (uint64, uint64, types.AccAddress, string) {
	return i.AccSeq, i.AccNum, i.PoolCosAddress, i.PoolPk
}

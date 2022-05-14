package common

import (
	"math/big"
	"strconv"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func (i *OutBoundReq) Hash() common.Hash {
	hash := crypto.Keccak256Hash(i.OutReceiverAddress.Bytes(), []byte(i.TxID))
	return hash
}

func NewOutboundReq(txID string, address, fromPoolAddr common.Address, coin types.Coin, coinAddr string, blockHeight, roundBlockHeight int64) OutBoundReq {
	return OutBoundReq{
		txID,
		address,
		fromPoolAddr,
		coin,
		coinAddr,
		roundBlockHeight,
		blockHeight,
		blockHeight,
		uint64(0),
	}
}

// Index generate the index of a given inbound req
func (i *OutBoundReq) Index() *big.Int {
	hash := crypto.Keccak256Hash(i.OutReceiverAddress.Bytes(), []byte(i.TxID))
	lower := hash.Big().String()
	higher := strconv.FormatInt(i.OriginalHeight, 10)
	indexStr := higher + lower

	ret, ok := new(big.Int).SetString(indexStr, 10)
	if !ok {
		panic("invalid to create the index")
	}
	return ret
}

// SetItemHeightAndNonce sets the block height of the tx
func (o *OutBoundReq) SetItemHeightAndNonce(roundBlockHeight, blockHeight int64, nonce uint64) {
	o.RoundBlockHeight = roundBlockHeight
	o.BlockHeight = blockHeight
	o.Nonce = nonce
}

// GetOutBoundInfo return the outbound tx info
func (o *OutBoundReq) GetOutBoundInfo() (common.Address, common.Address, string, *big.Int, int64, uint64) {
	return o.OutReceiverAddress, o.FromPoolAddr, o.CoinAddr, o.Coin.Amount.BigInt(), o.RoundBlockHeight, o.Nonce
}

func (i *InBoundReq) Hash() common.Hash {
	hash := crypto.Keccak256Hash(i.Address.Bytes(), i.TxID)
	return hash
}

// Index generate the index of a given inbound req
func (i *InBoundReq) Index() *big.Int {
	hash := crypto.Keccak256Hash(i.Address.Bytes(), i.TxID)
	lower := hash.Big().String()
	higher := strconv.FormatInt(i.OriginalHeight, 10)
	indexStr := higher + lower

	ret, ok := new(big.Int).SetString(indexStr, 10)
	if !ok {
		panic("invalid to create the index")
	}
	return ret
}

func NewAccountInboundReq(address types.AccAddress, toPoolAddr common.Address, coin types.Coin, txid []byte, blockHeight, roundBlockHeight int64) InBoundReq {
	return InBoundReq{
		address,
		txid,
		toPoolAddr,
		coin,
		blockHeight,
		blockHeight,
		roundBlockHeight,
		0,
		0,
		nil,
		"",
	}
}

// GetInboundReqInfo returns the info of the inbound transaction
func (acq *InBoundReq) GetInboundReqInfo() (types.AccAddress, common.Address, types.Coin, int64, int64) {
	return acq.Address, acq.ToPoolAddr, acq.Coin, acq.BlockHeight, acq.RoundBlockHeight
}

// SetAccountInfoAndHeight sets the block height of the tx
func (acq *InBoundReq) SetAccountInfoAndHeight(number, seq uint64, address types.AccAddress, pk string, roundBlockHeight int64) {
	acq.AccNum = number
	acq.AccSeq = seq
	acq.PoolJoltifyAddress = address
	acq.PoolPk = pk
	acq.RoundBlockHeight = roundBlockHeight
}

// GetAccountInfo returns the account number and seq
func (acq *InBoundReq) GetAccountInfo() (uint64, uint64, types.AccAddress, string) {
	return acq.AccSeq, acq.AccNum, acq.PoolJoltifyAddress, acq.PoolPk
}

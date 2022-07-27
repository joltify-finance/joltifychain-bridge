package common

import (
	"encoding/hex"
	"math/big"
	"strconv"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func (i *OutBoundReq) Hash() common.Hash {
	data, err := hex.DecodeString(i.TxID)
	if err != nil {
		panic(err)
	}
	hash := crypto.Keccak256Hash(i.OutReceiverAddress.Bytes(), data)
	return hash
}

func NewOutboundReq(txID string, address, fromPoolAddr common.Address, coin types.Coin, coinAddr string, txBlockHeight int64) OutBoundReq {
	return OutBoundReq{
		txID,
		address,
		fromPoolAddr,
		coin,
		coinAddr,
		txBlockHeight,
		uint64(0),
	}
}

// Index generate the index of a given inbound req
func (i *OutBoundReq) Index() *big.Int {
	data, err := hex.DecodeString(i.TxID)
	if err != nil {
		panic(err)
	}
	hash := crypto.Keccak256Hash(i.OutReceiverAddress.Bytes(), data)
	lower := hash.Big().String()
	higher := strconv.FormatInt(i.BlockHeight, 10)
	indexStr := higher + lower

	ret, ok := new(big.Int).SetString(indexStr, 10)
	if !ok {
		panic("invalid to create the index")
	}
	return ret
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
	hash := crypto.Keccak256Hash(i.Address.Bytes(), i.TxID)
	return hash
}

// Index generate the index of a given inbound req
func (i *InBoundReq) Index() *big.Int {
	hash := crypto.Keccak256Hash(i.Address.Bytes(), i.TxID)
	lower := hash.Big().String()
	higher := strconv.FormatInt(i.BlockHeight, 10)
	indexStr := higher + lower

	ret, ok := new(big.Int).SetString(indexStr, 10)
	if !ok {
		panic("invalid to create the index")
	}
	return ret
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
func (acq *InBoundReq) GetInboundReqInfo() (types.AccAddress, common.Address, types.Coin, int64) {
	return acq.Address, acq.ToPoolAddr, acq.Coin, acq.BlockHeight
}

// SetAccountInfo sets the block height of the tx
func (acq *InBoundReq) SetAccountInfo(number, seq uint64, address types.AccAddress, pk string) {
	acq.AccNum = number
	acq.AccSeq = seq
	acq.PoolOppyAddress = address
	acq.PoolPk = pk
}

// GetAccountInfo returns the account number and seq
func (acq *InBoundReq) GetAccountInfo() (uint64, uint64, types.AccAddress, string) {
	return acq.AccSeq, acq.AccNum, acq.PoolOppyAddress, acq.PoolPk
}

package common

import (
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"strconv"
)

// OutBoundReq is the entity for the outbound tx
type OutBoundReq struct {
	TxID               string
	outReceiverAddress common.Address
	fromPoolAddr       common.Address
	coin               types.Coin
	BlockHeight        int64
	nonce              uint64
}

func (i *OutBoundReq) Hash() common.Hash {
	hash := crypto.Keccak256Hash(i.outReceiverAddress.Bytes(), []byte(i.TxID))
	return hash
}

func NewOutboundReq(txID string, address, fromPoolAddr common.Address, coin types.Coin, blockHeight int64) OutBoundReq {
	return OutBoundReq{
		txID,
		address,
		fromPoolAddr,
		coin,
		blockHeight,
		uint64(0),
	}
}

// Index generate the index of a given inbound req
func (i *OutBoundReq) Index() *big.Int {
	hash := crypto.Keccak256Hash(i.outReceiverAddress.Bytes(), []byte(i.TxID))
	lower := hash.Big().String()
	higher := strconv.FormatInt(i.BlockHeight, 10)
	indexStr := higher + lower

	ret, ok := new(big.Int).SetString(indexStr, 10)
	if !ok {
		panic("invalid to create the index")
	}
	return ret
}

// SetItemHeightandNonce sets the block height of the tx
func (o *OutBoundReq) SetItemHeightandNonce(blockHeight int64, nonce uint64) {
	o.BlockHeight = blockHeight
	o.nonce = nonce
}

// GetOutBoundInfo return the outbound tx info
func (o *OutBoundReq) GetOutBoundInfo() (common.Address, common.Address, *big.Int, int64, uint64) {
	return o.outReceiverAddress, o.fromPoolAddr, o.coin.Amount.BigInt(), o.BlockHeight, o.nonce
}

func (i *InboundReq) Hash() common.Hash {
	hash := crypto.Keccak256Hash(i.address.Bytes(), i.TxID)
	return hash
}

// Index generate the index of a given inbound req
func (i *InboundReq) Index() *big.Int {
	hash := crypto.Keccak256Hash(i.address.Bytes(), i.TxID)
	lower := hash.Big().String()
	higher := strconv.FormatInt(i.originalHeight, 10)
	indexStr := higher + lower

	ret, ok := new(big.Int).SetString(indexStr, 10)
	if !ok {
		panic("invalid to create the index")
	}
	return ret
}

func NewAccountInboundReq(address types.AccAddress, toPoolAddr common.Address, coin types.Coin, txid []byte, blockHeight int64) InboundReq {
	return InboundReq{
		address,
		txid,
		toPoolAddr,
		coin,
		blockHeight,
		blockHeight,
		0,
		0,
		nil,
		"",
	}
}

// InboundReq is the account that top up account info to joltify pub_chain
type InboundReq struct {
	address            types.AccAddress
	TxID               []byte // this indicates the identical inbound req
	toPoolAddr         common.Address
	coin               types.Coin
	blockHeight        int64
	originalHeight     int64
	accNum             uint64
	accSeq             uint64
	poolJoltifyAddress types.AccAddress
	poolPk             string
}

// GetInboundReqInfo returns the info of the inbound transaction
func (acq *InboundReq) GetInboundReqInfo() (types.AccAddress, common.Address, types.Coin, int64) {
	return acq.address, acq.toPoolAddr, acq.coin, acq.blockHeight
}

// SetItemHeight sets the block height of the tx
func (acq *InboundReq) SetItemHeight(blockHeight int64) {
	acq.blockHeight = blockHeight
}

// SetAccountInfo sets the block height of the tx
func (acq *InboundReq) SetAccountInfo(number, seq uint64, address types.AccAddress, pk string) {
	acq.accNum = number
	acq.accSeq = seq
	acq.poolJoltifyAddress = address
	acq.poolPk = pk
}

//GetAccountInfo returns the account number and seq
func (acq *InboundReq) GetAccountInfo() (uint64, uint64, types.AccAddress, string) {
	return acq.accSeq, acq.accNum, acq.poolJoltifyAddress, acq.poolPk
}

package common

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// OutBoundReq is the entity for the outbound tx
type OutBoundReq struct {
	TxID               string         `json:"tx_id"`
	OutReceiverAddress common.Address `json:"out_receiver_address"`
	FromPoolAddr       common.Address `json:"from_pool_addr"`
	Coin               types.Coin     `json:"Coin"`
	RoundBlockHeight   int64          `json:"round_block_height"`
	BlockHeight        int64          `json:"block_height"`
	OriginalHeight     int64          `json:"original_height"`
	Nonce              uint64         `json:"nonce"`
}

func (i *OutBoundReq) Hash() common.Hash {
	hash := crypto.Keccak256Hash(i.OutReceiverAddress.Bytes(), []byte(i.TxID))
	return hash
}

func (i *OutBoundReq) Show() string {
	str := fmt.Sprintf("debvug >>>%v>>>%v>>>%v", i.OutReceiverAddress.Hex(), i.TxID, i.OriginalHeight)
	return str
}

func NewOutboundReq(txID string, address, fromPoolAddr common.Address, coin types.Coin, blockHeight, roundBlockHeight int64) OutBoundReq {
	return OutBoundReq{
		txID,
		address,
		fromPoolAddr,
		coin,
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
func (o *OutBoundReq) GetOutBoundInfo() (common.Address, common.Address, *big.Int, int64, uint64) {
	return o.OutReceiverAddress, o.FromPoolAddr, o.Coin.Amount.BigInt(), o.RoundBlockHeight, o.Nonce
}

func (i *InboundReq) Hash() common.Hash {
	hash := crypto.Keccak256Hash(i.Address.Bytes(), i.TxID)
	return hash
}

// Index generate the index of a given inbound req
func (i *InboundReq) Index() *big.Int {
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

func NewAccountInboundReq(address types.AccAddress, toPoolAddr common.Address, coin types.Coin, txid []byte, blockHeight, roundBlockHeight int64) InboundReq {
	return InboundReq{
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

// InboundReq is the account that top up account info to joltify pub_chain
type InboundReq struct {
	Address            types.AccAddress `json:"address"`
	TxID               []byte           `json:"tx_id"` // this indicates the identical inbound req
	ToPoolAddr         common.Address   `json:"to_pool_addr"`
	Coin               types.Coin       `json:"coin"`
	BlockHeight        int64            `json:"block_height"`
	OriginalHeight     int64            `json:"original_height"`
	RoundBlockHeight   int64            `json:"round_block_height"`
	AccNum             uint64           `json:"acc_num"`
	AccSeq             uint64           `json:"acc_seq"`
	PoolJoltifyAddress types.AccAddress `json:"pool_joltify_address"`
	PoolPk             string           `json:"pool_pk"`
}

// GetInboundReqInfo returns the info of the inbound transaction
func (acq *InboundReq) GetInboundReqInfo() (types.AccAddress, common.Address, types.Coin, int64) {
	return acq.Address, acq.ToPoolAddr, acq.Coin, acq.BlockHeight
}

// SetItemHeight sets the block height of the tx
func (acq *InboundReq) SetItemHeight(blockHeight int64) {
	acq.BlockHeight = blockHeight
}

// SetAccountInfo sets the block height of the tx
func (acq *InboundReq) SetAccountInfo(number, seq uint64, address types.AccAddress, pk string) {
	acq.AccNum = number
	acq.AccSeq = seq
	acq.PoolJoltifyAddress = address
	acq.PoolPk = pk
}

// GetAccountInfo returns the account number and seq
func (acq *InboundReq) GetAccountInfo() (uint64, uint64, types.AccAddress, string) {
	return acq.AccSeq, acq.AccNum, acq.PoolJoltifyAddress, acq.PoolPk
}

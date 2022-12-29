package common

import (
	"sync"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	vaulttypes "github.com/joltify-finance/joltify_lending/x/vault/types"
)

// PoolInfo stores the pool and pk of the joltify pool
type PoolInfo struct {
	Pk         string               `json:"pool_pubkey"`
	CosAddress types.AccAddress     `json:"oppy_address"`
	EthAddress common.Address       `json:"eth_address"`
	PoolInfo   *vaulttypes.PoolInfo `json:"pool_info"`
	// the height is only useful in saving the tx
	Height int64 `json:"height"`
}

// OutBoundReq is the entity for the outbound tx
type OutBoundReq struct {
	TxID               string         `json:"tx_id"`
	OutReceiverAddress common.Address `json:"out_receiver_address"`
	FromPoolAddr       common.Address `json:"from_pool_addr"`
	Coin               types.Coin     `json:"Coin"`
	TokenAddr          string         `json:"coin_address"`
	BlockHeight        int64          `json:"block_height"`
	Nonce              uint64         `json:"nonce"`
	SubmittedTxHash    string         `json:"submitted_tx_hash"` // this item is used for checking whether it is accepted on pubchain
	FeeToValidator     types.Coins    `json:"fee_to_validator"`
	ChainType          string         `json:"chain_type"`
	NeedMint           bool           `json:"need_mint"`
}

// InBoundReq is the account that top up account info to joltify pub_chain
type InBoundReq struct {
	UserReceiverAddress types.AccAddress `json:"user_receiver_address"`
	TxID                []byte           `json:"tx_id"` // this indicates the identical inbound req
	ToPoolAddr          common.Address   `json:"to_pool_addr"`
	Coin                types.Coin       `json:"coin"`
	BlockHeight         int64            `json:"block_height"`
	AccNum              uint64           `json:"acc_num"`
	AccSeq              uint64           `json:"acc_seq"`
	PoolCosAddress      types.AccAddress `json:"pool_oppy_address"`
	PoolPk              string           `json:"pool_pk"`
}

type BridgeMemo struct {
	Dest      string `json:"dest"`
	USerData  string `json:"user_data"`
	ChainType string `json:"chain_type"`
}

type RetryPools struct {
	RetryOutboundReq *sync.Map // if a tx fail to process, we need to put in this channel and wait for retry
}

func NewRetryPools() *RetryPools {
	return &RetryPools{
		&sync.Map{},
	}
}

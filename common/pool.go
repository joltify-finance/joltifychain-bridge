package common

import (
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	vaulttypes "gitlab.com/joltify/joltifychain/x/vault/types"
)

// PoolInfo stores the pool and pk of the joltify pool
type PoolInfo struct {
	Pk             string
	JoltifyAddress types.AccAddress
	EthAddress     common.Address
	PoolInfo       *vaulttypes.PoolInfo
}

// OutBoundReq is the entity for the outbound tx
type OutBoundReq struct {
	TxID               string         `json:"tx_id"`
	OutReceiverAddress common.Address `json:"out_receiver_address"`
	FromPoolAddr       common.Address `json:"from_pool_addr"`
	Coin               types.Coin     `json:"Coin"`
	TokenAddr          string         `json:"coin_address"`
	RoundBlockHeight   int64          `json:"round_block_height"`
	BlockHeight        int64          `json:"block_height"`
	OriginalHeight     int64          `json:"original_height"`
	Nonce              uint64         `json:"nonce"`
}

// InBoundReq is the account that top up account info to joltify pub_chain
type InBoundReq struct {
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

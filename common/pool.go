package common

import (
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	vaulttypes "gitlab.com/oppy-finance/oppychain/x/vault/types"
)

// PoolInfo stores the pool and pk of the oppy pool
type PoolInfo struct {
	Pk          string               `json:"pool_pubkey"`
	OppyAddress types.AccAddress     `json:"oppy_address"`
	EthAddress  common.Address       `json:"eth_address"`
	PoolInfo    *vaulttypes.PoolInfo `json:"pool_info"`
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
	Fee                types.Coins    `json:"fee"`
	FeeToValidator     types.Coins    `json:"fee_to_validator"`
}

// InBoundReq is the account that top up account info to oppy pub_chain
type InBoundReq struct {
	Address         types.AccAddress `json:"address"`
	TxID            []byte           `json:"tx_id"` // this indicates the identical inbound req
	ToPoolAddr      common.Address   `json:"to_pool_addr"`
	Coin            types.Coin       `json:"coin"`
	BlockHeight     int64            `json:"block_height"`
	AccNum          uint64           `json:"acc_num"`
	AccSeq          uint64           `json:"acc_seq"`
	PoolOppyAddress types.AccAddress `json:"pool_oppy_address"`
	PoolPk          string           `json:"pool_pk"`
}

type BridgeMemo struct {
	Dest     string `json:"dest"`
	TopupID  string `json:"topup_id"`
	USerData string `json:"user_data"`
}

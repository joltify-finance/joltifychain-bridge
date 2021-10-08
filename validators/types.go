package validators

import (
	"github.com/cosmos/cosmos-sdk/types"
	"sync"
)

type Validator struct {
	Address     types.Address
	PubKey      []byte
	VotingPower int64
}
type ValidatorSet struct {
	locker           *sync.RWMutex
	activeValidators map[string]*Validator
	blockHeight      int64
}

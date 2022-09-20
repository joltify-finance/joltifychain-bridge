package validators

import (
	"sync"

	"github.com/cosmos/cosmos-sdk/types"
)

// Validator defines the entities for the validators
type Validator struct {
	Address     types.Address
	PubKey      []byte
	VotingPower int64
}

// ValidatorSet defines the set of the validators
type ValidatorSet struct {
	locker           *sync.RWMutex
	activeValidators map[string]*Validator
	blockHeight      int64
}

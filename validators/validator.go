package validators

import (
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"sync"
)

func NewValidator() *ValidatorSet {

	return &ValidatorSet{
		&sync.RWMutex{},
		nil,
		0,
	}
}

func (v *ValidatorSet) SetupValidatorSet(validators []*tmservice.Validator, blockHeight int64) {
	v.locker = &sync.RWMutex{}
	v.locker.Lock()
	defer v.locker.Unlock()
	v.blockHeight = blockHeight
	v.activeValidators = validators
}

func (v *ValidatorSet) UpdateValidatorSet(validatorUpdates []*tmservice.Validator, blockHeight int64) {
	v.locker.Lock()
	defer v.locker.Unlock()
	v.blockHeight = blockHeight
	v.activeValidators = validatorUpdates
}

func (v *ValidatorSet) GetActiveValidators() ([]*tmservice.Validator, int64) {
	v.locker.RLock()
	defer v.locker.RUnlock()
	activeValidators := make([]*tmservice.Validator, len(v.activeValidators))
	i := 0
	for _, el := range v.activeValidators {
		activeValidators[i] = el
		i += 1
	}
	return activeValidators, v.blockHeight
}

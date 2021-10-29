package validators

import (
	"sync"

	"github.com/cosmos/cosmos-sdk/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

//NewValidator initialize a validator set
func NewValidator() *ValidatorSet {
	return &ValidatorSet{
		&sync.RWMutex{},
		make(map[string]*Validator),
		0,
	}
}

//SetupValidatorSet set up the validator set
func (v *ValidatorSet) SetupValidatorSet(validators []*Validator, blockHeight int64) {
	v.locker = &sync.RWMutex{}
	v.locker.Lock()
	defer v.locker.Unlock()
	v.blockHeight = blockHeight

	for _, el := range validators {

		localVal := Validator{
			el.Address,
			el.PubKey,
			el.VotingPower,
		}
		v.activeValidators[el.Address.String()] = &localVal
	}
}

//UpdateValidatorSet updates the validator set
func (v *ValidatorSet) UpdateValidatorSet(validatorUpdates []*tmtypes.Validator, blockHeight int64) error {
	v.locker.Lock()
	defer v.locker.Unlock()
	v.blockHeight = blockHeight
	for _, el := range validatorUpdates {
		addr, err := types.ConsAddressFromHex(el.Address.String())
		if err != nil {
			return err
		}
		if el.VotingPower == 0 {
			delete(v.activeValidators, addr.String())
		} else {
			localVal := Validator{
				addr,
				el.PubKey.Bytes(),
				el.VotingPower,
			}
			v.activeValidators[addr.String()] = &localVal
		}
	}
	return nil
}

//GetActiveValidators get the active validators
func (v *ValidatorSet) GetActiveValidators() ([]*Validator, int64) {
	v.locker.RLock()
	defer v.locker.RUnlock()
	activeValidators := make([]*Validator, len(v.activeValidators))
	i := 0
	for _, el := range v.activeValidators {
		activeValidators[i] = el
		i++
	}
	return activeValidators, v.blockHeight
}

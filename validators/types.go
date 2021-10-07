package validators

import tmtypes "github.com/tendermint/tendermint/types"

type ValidatorSet struct {
	currentValidators []*tmtypes.Validator
	blockHeight       uint64
}

func NewValidatorSet() *ValidatorSet {
	return &ValidatorSet{
		make([]*tmtypes.Validator, 0),
		0,
	}
}

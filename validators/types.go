package validators

import (
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"sync"
)

type ValidatorSet struct {
	locker           *sync.RWMutex
	activeValidators []*tmservice.Validator
	blockHeight      int64
}

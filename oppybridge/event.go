package oppybridge

import (
	"encoding/base64"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/types/bech32/legacybech32" //nolint
	"github.com/oppyfinance/tss/keygen"
	"gitlab.com/oppy-finance/oppy-bridge/tssclient"
	"go.uber.org/atomic"

	"github.com/oppyfinance/tss/common"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
)

const (
	capacity = 10000
	maxretry = 5
)

var sleepTime = 60

// HandleUpdateValidators check whether we need to generate the new tss pool message
func (oc *OppyChainInstance) HandleUpdateValidators(height int64, wg *sync.WaitGroup, inKeygenProcess *atomic.Bool, mock bool) error {
	v, err := oc.getValidators(strconv.FormatInt(height, 10))
	if err != nil {
		return err
	}

	err = oc.UpdateLatestValidator(v, height)
	if err != nil {
		oc.logger.Error().Msgf("fail to query the latest validator %v", err)
		return err
	}
	lastValidators, blockHeight := oc.GetLastValidator()

	pubKeys := make([]string, len(lastValidators))
	doKeyGen := false
	for i, el := range lastValidators {
		key := ed25519.PubKey{
			Key: el.PubKey,
		}
		ret := &key

		pk, err := legacybech32.MarshalPubKey(legacybech32.AccPK, ret) //nolint
		if err != nil {
			return err
		}

		pkValidator, err := base64.StdEncoding.DecodeString(oc.myValidatorInfo.Result.ValidatorInfo.PubKey.Value)
		if err != nil {
			return err
		}

		myValidatorPubKey := ed25519.PubKey{
			Key: pkValidator,
		}

		if key.Equals(&myValidatorPubKey) {
			doKeyGen = true
		}
		pubKeys[i] = pk
	}
	if !doKeyGen {
		return nil
	}

	oc.logger.Info().Msgf(">>>>>>>>>>>>>>>>at block height %v system do keygen>>>>>>>>>>>>>>>\n", blockHeight)
	oc.logger.Info().Msgf("public keys: %v>>>>>>>>>>>>>>>\n", pubKeys)
	oc.logger.Info().Msgf(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>\n")

	var errKeygen error
	errWriteLock := sync.Mutex{}
	var keygenResponse *keygen.Response
	wg.Add(1)
	go func() {
		defer wg.Done()
		sort.Strings(pubKeys)
		inKeygenProcess.Store(true)
		defer inKeygenProcess.Store(false)
		retry := 0
		for retry = 0; retry < maxretry; retry++ {
			resp, err := oc.tssServer.KeyGen(pubKeys, blockHeight, tssclient.TssVersion)
			if err != nil {
				oc.logger.Error().Err(err).Msgf("fail to do the keygen with retry %v", retry)
				if mock {
					sleepTime = 1
				}
				time.Sleep(time.Second * time.Duration(sleepTime))
				continue
			}
			if resp.Status != common.Success {
				// todo we need to put our blame on pub_chain as well
				oc.logger.Error().Msgf("we fail to ge the valid key")
				if mock {
					sleepTime = 1
				}
				time.Sleep(time.Second * time.Duration(sleepTime))
				continue
			}
			keygenResponse = &resp
			break
		}

		if retry >= maxretry {
			errWriteLock.Lock()
			errKeygen = errors.New("fail to get the valid key")
			errWriteLock.Unlock()
			return
		}

		oc.logger.Info().Msgf("we done the keygen at height %v successfully\n", blockHeight)

		// now we put the tss key on pub_chain
		// fixme we need to allow user to choose the name of the key
		creator, err := oc.Keyring.Key("operator")
		if err != nil {
			oc.logger.Error().Msgf("fail to get the operator key :%v", err)
			errKeygen = err
			return
		}

		err = oc.prepareTssPool(creator.GetAddress(), keygenResponse.PubKey, strconv.FormatInt(blockHeight+1, 10))
		if err != nil {
			oc.logger.Error().Msgf("fail to broadcast the tss generated key on pub_chain")
			errKeygen = err
			return
		}
		oc.logger.Info().Msgf("successfully prepared the tss key info on pub_chain")
	}()
	errWriteLock.Lock()
	defer errWriteLock.Unlock()
	return errKeygen
}

package pubchain

import (
	"encoding/base64"
	"math/big"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/oppyfinance/tss/common"
	"github.com/oppyfinance/tss/keygen"
	"github.com/oppyfinance/tss/keysign"
	"github.com/stretchr/testify/assert"
	common2 "gitlab.com/oppy-finance/oppy-bridge/common"
)

type TssMock struct {
	sk *secp256k1.PrivKey
}

func (tm *TssMock) KeySign(pk string, msgs []string, blockHeight int64, signers []string, version string) (keysign.Response, error) {
	var sigs []keysign.Signature
	for i, el := range msgs {
		msg, err := base64.StdEncoding.DecodeString(el)
		if err != nil {
			return keysign.Response{}, err
		}

		sk, err := crypto.ToECDSA(tm.sk.Bytes())
		if err != nil {
			return keysign.Response{}, err
		}
		signature, err := crypto.Sign(msg, sk)
		if err != nil {
			return keysign.Response{}, err
		}
		r := signature[:32]
		s := signature[32:64]
		v := signature[64:65]

		rEncoded := base64.StdEncoding.EncodeToString(r)
		sEncoded := base64.StdEncoding.EncodeToString(s)
		vEncoded := base64.StdEncoding.EncodeToString(v)

		sig := keysign.Signature{
			Msg:        msgs[i],
			R:          rEncoded,
			S:          sEncoded,
			RecoveryID: vEncoded,
		}
		sigs = append(sigs, sig)
	}
	return keysign.Response{Signatures: sigs, Status: common.Success}, nil
}

func (tm *TssMock) KeyGen(keys []string, blockHeight int64, version string) (keygen.Response, error) {
	return keygen.Response{}, nil
}

func (tm *TssMock) GetTssNodeID() string {
	return ""
}

func (tm *TssMock) Stop() {
}

type sortInboundReq []*common2.InBoundReq

func (s sortInboundReq) Len() int {
	return len(s)
}

func (s sortInboundReq) Less(i, j int) bool {
	hi := s[i].Hash().Big()
	hj := s[j].Hash().Big()
	return hi.Cmp(hj) == 1
}

func createNreq(n int) ([]*common2.InBoundReq, []*common2.InBoundReq, error) {
	accs, err := generateRandomPrivKey(n + 1)
	if err != nil {
		return nil, nil, err
	}
	reqs := make([]*common2.InBoundReq, n)
	reqsSorted := make(sortInboundReq, n)
	for i := 0; i < n; i++ {
		req := common2.NewAccountInboundReq(accs[i].oppyAddr, accs[n].commAddr, sdk.NewCoin("test", sdk.NewInt(1)), []byte(strconv.Itoa(i)), int64(i))
		req2 := common2.NewAccountInboundReq(accs[i].oppyAddr, accs[n].commAddr, sdk.NewCoin("test", sdk.NewInt(1)), []byte(strconv.Itoa(i)), int64(i))
		reqs[i] = &req
		reqsSorted[i] = &req2
	}
	sort.Slice(reqsSorted, func(i, j int) bool {
		a := reqsSorted[i]
		b := reqsSorted[j]
		hash := crypto.Keccak256Hash(a.Address.Bytes(), a.TxID)
		lower := hash.Big().String()
		higher := strconv.FormatInt(a.BlockHeight, 10)
		indexStr := higher + lower
		aret, _ := new(big.Int).SetString(indexStr, 10)

		hash2 := crypto.Keccak256Hash(b.Address.Bytes(), b.TxID)
		lower2 := hash2.Big().String()
		higher2 := strconv.FormatInt(b.BlockHeight, 10)
		indexStr2 := higher2 + lower2
		bret, _ := new(big.Int).SetString(indexStr2, 10)
		return aret.Cmp(bret) == -1
	})
	return reqs, reqsSorted, nil
}

func TestCreateInstance(t *testing.T) {
	pi := Instance{
		lastTwoPools:    make([]*common2.PoolInfo, 2),
		poolLocker:      &sync.RWMutex{},
		RetryInboundReq: &sync.Map{}, // if a tx fail to process, we need to put in this channel and wait for retry
	}
	reqs, sortedReqs, err := createNreq(500)
	assert.Nil(t, err)
	for i := 0; i < len(sortedReqs); i++ {
		pi.AddItem(reqs[i])
	}

	// now we test whether the pop is in the correct order
	returnedReqs := pi.PopItem(len(sortedReqs))

	for i := 0; i < len(sortedReqs); i++ {
		assert.True(t, returnedReqs[i].Address.Equals(sortedReqs[i].Address))
	}
}

package pubchain

import (
	"encoding/base64"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joltify-finance/tss/blame"
	"github.com/joltify-finance/tss/common"
	"github.com/joltify-finance/tss/keygen"
	"github.com/joltify-finance/tss/keysign"
	"github.com/stretchr/testify/assert"
	common2 "gitlab.com/joltify/joltifychain-bridge/common"
)

type TssMock struct {
	sk *secp256k1.PrivKey
}

func (tm *TssMock) KeySign(pk string, msgs []string, blockHeight int64, signers []string, version string) (keysign.Response, error) {
	msg, err := base64.StdEncoding.DecodeString(msgs[0])
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
		msgs[0],
		rEncoded,
		sEncoded,
		vEncoded,
	}

	return keysign.Response{[]keysign.Signature{sig}, common.Success, blame.Blame{}}, nil
}

func (tm *TssMock) KeyGen(keys []string, blockHeight int64, version string) (keygen.Response, error) {
	return keygen.Response{}, nil
}

func (tm *TssMock) GetTssNodeID() string {
	return ""
}

func (tm *TssMock) Stop() {
	return
}

type sortInboundReq []*common2.InboundReq

func (s sortInboundReq) Len() int {
	return len(s)
}

func (s sortInboundReq) Less(i, j int) bool {
	hi := s[i].Hash().Big()
	hj := s[j].Hash().Big()
	return hi.Cmp(hj) == 1
}

func (s sortInboundReq) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func createNreq(n int) ([]*common2.InboundReq, []*common2.InboundReq, error) {
	accs, err := generateRandomPrivKey(n + 1)
	if err != nil {
		return nil, nil, err
	}
	reqs := make([]*common2.InboundReq, n)
	reqsSorted := make(sortInboundReq, n)
	for i := 0; i < n; i++ {
		req := common2.InboundReq{
			address:     accs[i].joltAddr,
			txID:        []byte(strconv.Itoa(i)), // this indicates the identical inbound req
			toPoolAddr:  accs[n].commAddr,
			coin:        sdk.NewCoin("test", sdk.NewInt(1)),
			blockHeight: int64(i),
		}
		reqs[i] = &req
		reqsSorted[i] = &req
	}
	sort.Stable(reqsSorted)
	return reqs, reqsSorted, nil
}

func TestCreateInstance(t *testing.T) {
	pi := PubChainInstance{
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

	for i := 0; i < len(sortedReqs); i++ {
		el := pi.PopItem()
		assert.True(t, el.address.Equals(sortedReqs[i].address))
	}
}

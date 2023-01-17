package tssclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/joltify-finance/tss/keygen"
	"github.com/joltify-finance/tss/keysign"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"gitlab.com/joltify/joltifychain-bridge/config"
	"gitlab.com/joltify/joltifychain-bridge/misc"

	. "gopkg.in/check.v1"
)

const (
	partyNum         = 4
	testFileLocation = "../test_data"
)

var testPubKeys = []string{
	"joltpub1zcjduepq8sysqavl67rtytlezccjkq4dvnewtscnlck3ku5uac46rggkfe8qdwyv3u",
	"joltpub1zcjduepqfa9cy9w6sn5wkzfm0zm25e7u9nag55ev4vmasqp7xxhlj78xz7kq2zcl6h",
	"joltpub1zcjduepqv4p9n26ss3djrnvwufjrs6v8gg5tr2fqpjv3av4n6tx4efclp2tqfxv5nm",
	"joltpub1zcjduepqaxxpz2kfr939dlyhdvuhqvmg2rkv8enls663uegnfqaghp9tr32szlgtm2",
}

func TestPackage(t *testing.T) {
	TestingT(t)
}

type FourNodeTestSuite struct {
	servers []*BridgeTssServer
}

var _ = Suite(&FourNodeTestSuite{})

// setup four nodes for test
func (s *FourNodeTestSuite) SetUpTest(c *C) {
	misc.SetupBech32Prefix()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	s.servers = make([]*BridgeTssServer, partyNum)

	conf := config.TssConfig{
		KeyGenTimeout:   90 * time.Second,
		KeySignTimeout:  90 * time.Second,
		PreParamTimeout: 60 * time.Second,
	}

	var wg sync.WaitGroup
	for i := 0; i < partyNum; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.servers[idx] = s.getTssServer(c, idx, conf)
		}(i)
		time.Sleep(time.Second)
	}
	wg.Wait()
}

func hash(payload []byte) []byte {
	h := sha256.New()
	h.Write(payload)
	return h.Sum(nil)
}

// we do for both join party schemes
func (s *FourNodeTestSuite) Test4NodesTss(c *C) {
	s.doTestKeygenAndKeySign(c)
	fmt.Printf("not ready to run")
}

func checkSignResult(c *C, keysignResult map[int]keysign.Response) {
	for i := 0; i < len(keysignResult)-1; i++ {
		currentSignatures := keysignResult[i].Signatures
		// we test with two messsages and the size of the signature should be 44
		c.Assert(currentSignatures, HasLen, 2)
		c.Assert(currentSignatures[0].S, HasLen, 44)
		currentData, err := json.Marshal(currentSignatures)
		c.Assert(err, IsNil)
		nextSignatures := keysignResult[i+1].Signatures
		nextData, err := json.Marshal(nextSignatures)
		c.Assert(err, IsNil)
		ret := bytes.Equal(currentData, nextData)
		c.Assert(ret, Equals, true)
	}
}

// generate a new key
func (s *FourNodeTestSuite) doTestKeygenAndKeySign(c *C) {
	wg := sync.WaitGroup{}
	lock := &sync.Mutex{}
	keygenResult := make(map[int]keygen.Response)
	for i := 0; i < partyNum; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			localPubKeys := append([]string{}, testPubKeys...)
			res, err := s.servers[idx].KeyGen(localPubKeys, 10, "0.15.0")
			c.Assert(err, IsNil)
			lock.Lock()
			defer lock.Unlock()
			keygenResult[idx] = res
		}(i)
	}
	wg.Wait()

	var poolPubKey string
	for _, item := range keygenResult {
		if len(poolPubKey) == 0 {
			poolPubKey = item.PubKey
		} else {
			c.Assert(poolPubKey, Equals, item.PubKey)
		}
	}

	resp, err := s.servers[0].KeySign(poolPubKey, []string{"helloworld", "helloworld2"}, 10, testPubKeys, "0.14.0")
	c.Assert(err, NotNil)
	c.Assert(resp.Signatures, HasLen, 0)

	keysignResult := make(map[int]keysign.Response)
	for i := 0; i < partyNum; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			localPubKeys := append([]string{}, testPubKeys...)
			res, err := s.servers[idx].KeySign(poolPubKey, []string{base64.StdEncoding.EncodeToString(hash([]byte("helloworld"))), base64.StdEncoding.EncodeToString(hash([]byte("helloworld2")))}, 10, localPubKeys, "0.14.0")
			c.Assert(err, IsNil)
			lock.Lock()
			defer lock.Unlock()
			keysignResult[idx] = res
		}(i)
	}
	wg.Wait()
	checkSignResult(c, keysignResult)

	keysignResult1 := make(map[int]keysign.Response)
	for i := 0; i < partyNum; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			res, err := s.servers[idx].KeySign(poolPubKey, []string{base64.StdEncoding.EncodeToString(hash([]byte("helloworld"))), base64.StdEncoding.EncodeToString(hash([]byte("helloworld2")))}, 10, nil, "0.14.0")
			c.Assert(err, IsNil)
			lock.Lock()
			defer lock.Unlock()
			keysignResult1[idx] = res
		}(i)
	}
	wg.Wait()
	checkSignResult(c, keysignResult1)
}

func (s *FourNodeTestSuite) TearDownTest(c *C) {
	// give a second before we shutdown the network
	time.Sleep(time.Second)
	for i := 0; i < partyNum; i++ {
		s.servers[i].Stop()
	}
	for i := 0; i < partyNum; i++ {
		tempFilePath := path.Join(os.TempDir(), "4nodes_test", strconv.Itoa(i))
		os.RemoveAll(tempFilePath)

	}
}

func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

func (s *FourNodeTestSuite) getTssServer(c *C, index int, conf config.TssConfig) *BridgeTssServer {
	baseHome := path.Join(os.TempDir(), "4nodes_test", strconv.Itoa(index))
	if _, err := os.Stat(baseHome); os.IsNotExist(err) {
		err := os.MkdirAll(baseHome, os.ModePerm)
		c.Assert(err, IsNil)
	}

	p2, err1 := os.Getwd()
	c.Assert(err1, IsNil)

	src := path.Join(p2, testFileLocation, "keys", strconv.Itoa(index), "priv_validator_key.json")
	dst := path.Join(baseHome, "priv_validator_key.json")

	_, err := copy(src, dst)
	c.Assert(err, IsNil)

	if index != 0 {
		bootstrap, err := maddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1667/p2p/12D3KooWF9uA3Tib6hkeNqwPBNH9FC5ncgurZzpfKKXbXNXuGHfy")
		c.Assert(err, IsNil)
		conf.BootstrapPeers = config.AddrList{bootstrap}
		conf.Port = 1667 + index
	} else {
		conf.Port = 1667
	}

	instance, _, err := StartTssServer(baseHome, conf)
	// instance, err := NewTss(peerIDs, s.ports[index], priKey, "Asgard", baseHome, conf, s.preParams[index], "")
	c.Assert(err, IsNil)
	str := instance.GetTssNodeID()
	if len(str) == 0 {
		c.FailNow()
	}
	return instance
}

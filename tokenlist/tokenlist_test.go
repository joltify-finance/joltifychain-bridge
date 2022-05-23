package tokenlist

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

// var (
// 	testKey, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
// 	testAddr    = crypto.PubkeyToAddress(testKey.PublicKey)
// 	testBalance = big.NewInt(200e15)
// )

// var genesis = &core.Genesis{
// 	Config:    params.AllEthashProtocolChanges,
// 	Alloc:     core.GenesisAlloc{testAddr: {Balance: testBalance}},
// 	ExtraData: []byte("test genesis"),
// 	Timestamp: 9000,
// 	BaseFee:   big.NewInt(1),
// }

// func generateTestChain(testTx []*ethTypes.Transaction) []*ethTypes.Block {
// 	db := rawdb.NewMemoryDatabase()
// 	generate := func(i int, g *core.BlockGen) {
// 		g.OffsetTime(5)
// 		g.SetExtra([]byte("test"))
// 		if i == 1 {
// 			for _, el := range testTx {
// 				g.AddTx(el)
// 			}
// 		}
// 	}
// 	gblock := genesis.ToBlock(db)
// 	engine := ethash.NewFaker()
// 	blocks, _ := core.GenerateChain(genesis.Config, gblock, engine, db, len(testTx), generate)
// 	blocks = append([]*ethTypes.Block{gblock}, blocks...)
// 	return blocks
// }

// func newTestBackend(t *testing.T, txs []*ethTypes.Transaction) (*node.Node, []*ethTypes.Block) {
// 	// Generate test chain.

// 	blocks := generateTestChain(txs)

// 	// Create node
// 	n, err := node.New(&node.Config{})
// 	if err != nil {
// 		t.Fatalf("can't create new node: %v", err)
// 	}
// 	// Create Ethereum Service
// 	config := &ethconfig.Config{Genesis: genesis}
// 	config.Ethash.PowMode = ethash.ModeFake
// 	ethservice, err := eth.New(n, config)
// 	if err != nil {
// 		t.Fatalf("can't create new ethereum service: %v", err)
// 	}
// 	// Import the test chain.
// 	if err := n.Start(); err != nil {
// 		t.Fatalf("can't start test node: %v", err)
// 	}
// 	if _, err := ethservice.BlockChain().InsertChain(blocks[1:]); err != nil {
// 		t.Fatalf("can't import test blocks: %v", err)
// 	}
// 	return n, blocks
// }

// func createMockEthClient(t *testing.T) *ethclient.Client {
// 	backend, _ := newTestBackend(t, []*ethTypes.Transaction{})
// 	defer backend.Close()
// 	client, err := backend.Attach()
// 	assert.NilError(t, err)
// 	defer client.Close()

// 	c := ethclient.NewClient(client)
// 	return c
// }

func getTokenListFolderPath() string {
	current, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	tokenlistFolderPath := path.Join(current, "../test_data/tokenlist")
	return tokenlistFolderPath
}

func getTestUpdateTokenListFolderPath() string {
	current, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	tokenlistPath := path.Join(current, "../test_data/tokenlist_update")
	return tokenlistPath
}

func TestNewTokenList(t *testing.T) {
	tokenlistPath := getTokenListFolderPath()
	// client := createMockEthClient(t)
	// tl, err := NewTokenList(tokenlistPath, 100, client)
	tl, err := NewTokenList(tokenlistPath, 100)
	assert.NilError(t, err)
	assert.Equal(t, tl.UpdateGap, int64(100))

	// check tl.JoltTokenlist
	var exit bool
	var data interface{}
	var tokenAddr string
	var tokenDenom string
	data, exit = tl.JoltTokenList.Load("JUSD")
	tokenAddr = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenAddr, "0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")
	data, exit = tl.JoltTokenList.Load("JoltBNB")
	tokenAddr = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenAddr, "0x15fb343d82cD1C22542261dF408dA8396A829F6B")

	// check tl.PubTokenlist
	data, exit = tl.PubTokenList.Load("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JUSD")
	data, exit = tl.PubTokenList.Load("0x15fb343d82cD1C22542261dF408dA8396A829F6B")
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JoltBNB")

	// check tl.HistoryTokenList
	data, exit = tl.HistoryTokenList["histAddr1"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "histDenom1")
	data, exit = tl.HistoryTokenList["histAddr2"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "histDenom2")
	data, exit = tl.HistoryTokenList["0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JUSD")
	data, exit = tl.HistoryTokenList["0x15fb343d82cD1C22542261dF408dA8396A829F6B"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JoltBNB")
}

func TestUpdateTokenList(t *testing.T) {
	tokenlistPath := getTokenListFolderPath()
	// client := createMockEthClient(t)
	// tl, err := NewTokenList(tokenlistPath, 100, client)
	tl, err := NewTokenList(tokenlistPath, 100)
	assert.NilError(t, err)
	assert.Equal(t, tl.UpdateGap, int64(100))

	tokenlistPathUpdate := getTestUpdateTokenListFolderPath()
	tl.FolderPath = tokenlistPathUpdate
	tl.UpdateTokenList(int64(100))
	assert.Equal(t, tl.UpdateMark, int64(1))

	var exit bool
	var data interface{}
	var tokenAddr string
	var tokenDenom string
	// check tl.JoltTokenlist
	_, exit = tl.JoltTokenList.Load("JUSD")
	assert.Equal(t, exit, false)
	data, exit = tl.JoltTokenList.Load("JoltBNB")
	tokenAddr = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenAddr, "0x15fb343d82cD1C22542261dF408dA8396A829F6B")
	data, exit = tl.JoltTokenList.Load("testUpdateDenom")
	tokenAddr = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenAddr, "testUpdateAddress")

	// check tl.PubTokenlist
	_, exit = tl.PubTokenList.Load("0xeB42ff4cA651c91EB248f8923358b6144c6B4b79")
	assert.Equal(t, exit, false)
	data, exit = tl.PubTokenList.Load("0x15fb343d82cD1C22542261dF408dA8396A829F6B")
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JoltBNB")
	data, exit = tl.PubTokenList.Load("testUpdateAddress")
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "testUpdateDenom")

	// check tl.HistoryTokenList
	data, exit = tl.HistoryTokenList["testUpdateAddress"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "testUpdateDenom")
	data, exit = tl.HistoryTokenList["histAddr1"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "histDenom1")
	data, exit = tl.HistoryTokenList["histAddr2"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "histDenom2")
	data, exit = tl.HistoryTokenList["0xeB42ff4cA651c91EB248f8923358b6144c6B4b79"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JUSD")
	data, exit = tl.HistoryTokenList["0x15fb343d82cD1C22542261dF408dA8396A829F6B"]
	tokenDenom = data.(string)
	assert.Equal(t, exit, true)
	assert.Equal(t, tokenDenom, "JoltBNB")
}

func TestExportHistoryTokenList(t *testing.T) {
	tokenlistPathUpdate := getTestUpdateTokenListFolderPath()
	tokenlist_history := make(map[string]string)
	tokenlist_history["testAddr"] = "testDenom"
	tl := TokenList{
		FolderPath:       tokenlistPathUpdate,
		HistoryTokenList: tokenlist_history,
	}
	err := tl.ExportHistoryTokenList()
	assert.NilError(t, err)

	//
	filePath := filepath.Join(tokenlistPathUpdate, "tokenlist_history.json")
	dat, err := ioutil.ReadFile(filePath)
	assert.NilError(t, err)
	result := make(map[string]string)
	json.Unmarshal([]byte(dat), &result)
	assert.Equal(t, len(result), 1)
	assert.Equal(t, result["testAddr"], "testDenom")

	// remove the test tokenlist_history.json file
	e := os.Remove(filePath)
	assert.NilError(t, e)
}

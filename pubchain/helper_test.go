package pubchain

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
	"gitlab.com/joltify/joltifychain-bridge/misc"
)

type TestHelperSuite struct {
	suite.Suite
	pubChain *Instance
}

func (tn *TestHelperSuite) SetupSuite() {
	misc.SetupBech32Prefix()

	wg := sync.WaitGroup{}
	pubChain, err := NewChainInstance(misc.WebsocketTest, misc.WebsocketTest, nil, nil, &wg, nil)
	if err != nil {
		panic(err)
	}
	tn.pubChain = pubChain
}

func (tn TestHelperSuite) TestAllWithLockOperations() {
	err := tn.pubChain.CheckPubChainHealthWithLock()
	tn.Require().NoError(err)
	_, err = tn.pubChain.BSCChain.GetBlockByNumberWithLock(nil)
	tn.Require().NoError(err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	ethAddr := common.HexToAddress("0xbDf7Fb0Ad9b0D722ea54D808b79751608E7AE991")
	balance, err := tn.pubChain.BSCChain.getBalanceWithLock(ctx, ethAddr)
	tn.Require().NoError(err)
	fmt.Printf(">>>>>>>>>>>>>>>>>>>>>>>>>%v\n", balance.String())
	tn.Require().True(balance.Cmp(big.NewInt(0)) == 1)

	nonce, err := tn.pubChain.BSCChain.getPendingNonceWithLock(ctx, ethAddr)
	tn.Require().NoError(err)
	tn.Require().True(nonce > 0)
	ethClient, err := ethclient.Dial(misc.WebsocketTest)
	tn.Require().NoError(err)
	err = tn.pubChain.BSCChain.renewEthClientWithLock(ethClient)
	tn.Require().NoError(err)

	_, err = tn.pubChain.BSCChain.GetGasPriceWithLock()
	tn.Require().NoError(err)

	// now we test the operations with closed client to focue renew
	tn.pubChain.BSCChain.Client.Close()
	balance, err = tn.pubChain.BSCChain.getBalanceWithLock(ctx, ethAddr)
	tn.Require().Error(err)
	tn.pubChain.wg.Wait()
	_, err = tn.pubChain.BSCChain.getBalanceWithLock(ctx, ethAddr)
	tn.Require().NoError(err)

	tn.pubChain.BSCChain.Client.Close()
	_, err = tn.pubChain.BSCChain.getPendingNonceWithLock(ctx, ethAddr)
	tn.Require().Error(err)
	tn.pubChain.wg.Wait()

	_, err = tn.pubChain.BSCChain.getBalanceWithLock(ctx, ethAddr)
	tn.Require().NoError(err)

	tn.pubChain.BSCChain.Client.Close()
	_, _, _, _, err = tn.pubChain.BSCChain.GetFeeLimitWithLock()
	tn.Require().Error(err)
	tn.pubChain.wg.Wait()
	_, _, _, _, err = tn.pubChain.BSCChain.GetFeeLimitWithLock()
	tn.Require().NoError(err)

	tn.pubChain.BSCChain.Client.Close()
	_, err = tn.pubChain.BSCChain.GetBlockByNumberWithLock(nil)
	tn.Require().Error(err)
	tn.pubChain.wg.Wait()
	_, err = tn.pubChain.BSCChain.GetBlockByNumberWithLock(nil)
	tn.Require().NoError(err)

	tn.pubChain.BSCChain.Client.Close()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	_, err = tn.pubChain.BSCChain.getTransactionReceiptWithLock(ctx, common.HexToHash("0xac0E15a038eedfc68ba3C35c73feD5bE4A07afB5"))
	tn.Require().Error(err)
	tn.pubChain.wg.Wait()
	_, err = tn.pubChain.BSCChain.getTransactionReceiptWithLock(ctx, common.HexToHash("0x6e1a0257370db7334ffc10c87d22e78ac0e3edf7a957ce61eb11e59c79300217"))
	tn.Require().NoError(err)

	tn.pubChain.BSCChain.Client.Close()
	tn.pubChain.HealthCheckAndReset()
	err = tn.pubChain.CheckPubChainHealthWithLock()
	tn.Require().NoError(err)
}

func (tn TestHelperSuite) TestRecoverKeyFromTx() {
	tn.pubChain.BSCChain.ChainLocker.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	h := common.HexToHash("0xf6a04ff4be84c163fb0e400848cff62bad19b6e757339705634e136495dfed3d")
	tx, _, err := tn.pubChain.BSCChain.Client.TransactionByHash(ctx, h)
	tn.Require().NoError(err)
	tn.pubChain.BSCChain.ChainLocker.Unlock()

	addr, err := tn.pubChain.retrieveAddrfromRawTx(tx)
	tn.Require().NoError(err)
	tn.Require().Equal("jolt196vj6jyqaqydjfqm5n58zegtrx2x02gcm6y9dt", addr.String())
}

func TestHelper(t *testing.T) {
	suite.Run(t, new(TestHelperSuite))
}

package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/crypto"
	"gitlab.com/oppy-finance/oppy-bridge/common"
	"gitlab.com/oppy-finance/oppy-bridge/storage"
)

func main() {
	fsm := storage.NewTxStateMgr("./")
	// now we load the existing outbound requests

	// now we load the existing inbound requests
	itemsIn, err := fsm.LoadInBoundState()
	if err != nil {
		fmt.Printf("we do not need to have the items to be loaded")
	}

	exportedInboundReq := []*common.InBoundReq{itemsIn[0]}
	balance := make(map[string]int64)
	balance[itemsIn[0].Coin.Denom] = itemsIn[0].Coin.Amount.Int64()
	names := []string{"abnb", "abusd"}
	r := rand.New(rand.NewSource(time.Now().Unix()))
	for i := 0; i < 100; i++ {
		acc, err := types.AccAddressFromBech32(itemsIn[0].Address.String())
		if err != nil {
			panic(err)
		}
		fmt.Printf("add amount %v\n", itemsIn[0].Coin.Amount.String())
		coin := types.Coin{
			Denom:  names[r.Intn(2)],
			Amount: itemsIn[0].Coin.Amount,
		}

		txID := crypto.Sha256([]byte(strconv.Itoa(i)))
		height := int64(0)
		for {
			height = r.Int63n(itemsIn[0].BlockHeight)
			if height > 100 {
				break
			}
		}
		item := common.NewAccountInboundReq(acc, itemsIn[0].ToPoolAddr, coin, txID, height)
		exportedInboundReq = append(exportedInboundReq, &item)

		current, ok := balance[item.Coin.Denom]
		if !ok {
			balance[item.Coin.Denom] = item.Coin.Amount.Int64()
		} else {
			balance[item.Coin.Denom] = current + item.Coin.Amount.Int64()
		}
	}

	fmt.Printf(">>>>>>>>>>>>>total\n")
	for key, val := range balance {
		fmt.Printf("%v>>>>>>>>>>>>%v\n", key, val)
	}

	fsm.SaveInBoundState(exportedInboundReq)
}

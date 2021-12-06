package pubchain

import (
	"github.com/ethereum/go-ethereum/common"
	"testing"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
)

func SetupBech32Prefix() {
	config := types.GetConfig()
	// thorchain will import go-tss as a library , thus this is not needed, we copy the prefix here to avoid go-tss to import thorchain
	config.SetBech32PrefixForAccount("jolt", "joltpub")
	config.SetBech32PrefixForValidator("joltval", "joltvpub")
	config.SetBech32PrefixForConsensusNode("joltvalcons", "joltcpub")
}

func TestAccount_Verify(t *testing.T) {
	SetupBech32Prefix()
	type fields struct {
		address   common.Address
		direction direction
		token     types.Coin
		fee       types.Coin
	}

	addr, err := types.AccAddressFromBech32("jolt1ljh48799pqcnezpsjr69ukpfq4mgapvpr7kzhm")
	assert.Nil(t, err)
	hexAddr := common.BytesToAddress(addr.Bytes())

	minFee, err := types.NewDecFromStr(inBoundFeeMin)
	assert.Nil(t, err)
	deltaFee, err := types.NewDecFromStr("0.00001")
	assert.Nil(t, err)
	larger := minFee.Add(deltaFee)
	small := minFee.Sub(deltaFee)

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "test ok",
			fields: fields{
				address:   hexAddr,
				direction: inBound,
				fee:       types.Coin{Denom: inBoundDenom, Amount: types.NewIntFromBigInt(larger.BigInt())},
			},
			wantErr: false,
		},
		{
			name: "not enough fee",
			fields: fields{
				address:   hexAddr,
				direction: inBound,
				fee:       types.Coin{Denom: inBoundDenom, Amount: types.NewIntFromBigInt(small.BigInt())},
			},
			wantErr: true,
		},
		{
			name: "wrong demon",
			fields: fields{
				address:   hexAddr,
				direction: inBound,
				fee:       types.Coin{Denom: "wrong", Amount: types.NewIntFromBigInt(small.BigInt())},
			},
			wantErr: true,
		},

		{
			name: "exact fee",
			fields: fields{
				address:   hexAddr,
				direction: inBound,
				fee:       types.Coin{Denom: "wrong", Amount: types.NewIntFromBigInt(minFee.BigInt())},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &inboundTx{
				address:   tt.fields.address,
				direction: tt.fields.direction,
				token:     tt.fields.token,
				fee:       tt.fields.fee,
			}
			if err := a.Verify(); (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

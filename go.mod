module invoicebridge

go 1.16

require (
	github.com/cosmos/cosmos-sdk v0.42.6
	github.com/froyobin/invoiceChain v0.0.0-00010101000000-000000000000
	github.com/ipfs/go-log v1.0.4
	github.com/joltgeorge/tss v1.3.1-0.20211004005605-8fdfeac26b65
	github.com/libp2p/go-libp2p-peerstore v0.2.6
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/rs/zerolog v1.21.0
	github.com/tendermint/tendermint v0.34.11
	google.golang.org/grpc v1.41.0
)

replace (
	github.com/binance-chain/tss-lib => gitlab.com/thorchain/tss/tss-lib v0.0.0-20201118045712-70b2cb4bf916
	github.com/froyobin/invoiceChain => /Users/yb/development/joltify/chain/invoiceChain
	github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1
	google.golang.org/grpc => google.golang.org/grpc v1.33.2
)

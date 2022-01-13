module gitlab.com/joltify/joltifychain-bridge

go 1.16

require (
	github.com/cosmos/cosmos-sdk v0.44.2
	github.com/ethereum/go-ethereum v1.10.11
	github.com/gorilla/mux v1.8.0
	github.com/ipfs/go-log v1.0.4
	github.com/joltgeorge/tss v1.3.1-0.20220114060604-a1b6c3b40131
	github.com/libp2p/go-libp2p-peerstore v0.2.6
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/prometheus/client_golang v1.11.0
	github.com/rs/zerolog v1.23.0
	github.com/stretchr/testify v1.7.0
	github.com/tendermint/btcd v0.1.1
	github.com/tendermint/tendermint v0.34.14
	gitlab.com/joltify/joltifychain v0.0.0-20220114010643-2d3dfce706a2
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5
	google.golang.org/grpc v1.43.0
)

replace (
	github.com/binance-chain/tss-lib => gitlab.com/thorchain/tss/tss-lib v0.0.0-20201118045712-70b2cb4bf916
	github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1
	google.golang.org/grpc => google.golang.org/grpc v1.33.2
)

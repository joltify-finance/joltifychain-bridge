package config

import (
	"flag"
	"strings"
	"time"

	maddr "github.com/multiformats/go-multiaddr"
)

type JoltifyChainConfig struct {
	GrpcAddress string
	WsAddress   string
	WsEndpoint  string
	HTTPAddress string
	RollbackGap int
}

type PubChainConfig struct {
	WsAddress   string
	RollbackGap int
}

type (
	AddrList  []maddr.Multiaddr
	TssConfig struct {
		// Party Timeout defines how long do we wait for the party to form
		PartyTimeout time.Duration
		// KeyGenTimeoutSeconds defines how long do we wait the keygen parties to pass messages along
		KeyGenTimeout time.Duration
		// KeySignTimeoutSeconds defines how long do we wait keysign
		KeySignTimeout time.Duration
		// Pre-parameter define the pre-parameter generations timeout
		PreParamTimeout time.Duration
		// enable the tss monitor

		// Config is configuration for Tss P2P
		RendezvousString string
		Port             int
		BootstrapPeers   AddrList
		ExternalIP       string
		HTTPAddr         string
	}
)

// String implement fmt.Stringer
func (al *AddrList) String() string {
	addresses := make([]string, len(*al))
	for i, addr := range *al {
		addresses[i] = addr.String()
	}
	return strings.Join(addresses, ",")
}

// Set add the given value to addList
func (al *AddrList) Set(value string) error {
	addr, err := maddr.NewMultiaddr(value)
	if err != nil {
		return err
	}
	*al = append(*al, addr)
	return nil
}

type Config struct {
	JoltifyChain       JoltifyChainConfig
	PubChainConfig     PubChainConfig
	TssConfig          TssConfig
	KeyringAddress     string
	HomeDir            string
	TokenListPath      string
	TokenListUpdateGap int
	EnableMonitor      bool
}

func DefaultConfig() Config {
	var config Config
	flag.StringVar(&config.JoltifyChain.GrpcAddress, "grpc-port", "127.0.0.1:9090", "address for joltify pub_chain")
	flag.StringVar(&config.JoltifyChain.WsAddress, "ws-port", "tcp://localhost:26657", "ws address for joltify pub_chain")
	flag.StringVar(&config.JoltifyChain.HTTPAddress, "http-port", "http://localhost:26657", "ws address for joltify pub_chain")
	flag.StringVar(&config.JoltifyChain.WsEndpoint, "ws-endpoint", "/websocket", "endpoint for joltify pub_chain")
	flag.IntVar(&config.JoltifyChain.RollbackGap, "joltify-rollback-gap", 10, "delay the transaction process to prevent chain rollback")
	flag.StringVar(&config.PubChainConfig.WsAddress, "pub-ws-endpoint", "ws://rpc.joltify.io:8456/", "endpoint for public pub_chain listener")
	flag.IntVar(&config.PubChainConfig.RollbackGap, "pubchain-rollback-gap", 10, "delay the transaction process to prevent chain rollback")
	flag.StringVar(&config.KeyringAddress, "key", "./keyring.key", "operator key path")
	flag.StringVar(&config.HomeDir, "home", "/root/.joltifyChain/config", "home director for joltify_bridge")
	flag.StringVar(&config.TokenListPath, "token-list", "tokenlist.json", "file path to load token white list")
	flag.IntVar(&config.TokenListUpdateGap, "tokenlist-update-gap", 30, "gap to update the token list")
	flag.StringVar(&config.TssConfig.HTTPAddr, "tss-http-port", "0.0.0.0:8321", "tss http port for info only")

	// we setup the Tss parameter configuration
	flag.DurationVar(&config.TssConfig.KeyGenTimeout, "gentimeout", 30*time.Second, "keygen timeout")
	flag.DurationVar(&config.TssConfig.KeySignTimeout, "signtimeout", 30*time.Second, "keysign timeout")
	flag.DurationVar(&config.TssConfig.PartyTimeout, "joinpartytimeout", 30*time.Second, "join party timeout")

	flag.DurationVar(&config.TssConfig.PreParamTimeout, "preparamtimeout", 5*time.Minute, "pre-parameter generation timeout")
	flag.BoolVar(&config.EnableMonitor, "enablemonitor", true, "enable the joltifyChain monitor")

	// we setup the p2p network configuration
	flag.StringVar(&config.TssConfig.RendezvousString, "rendezvous", "joltifyChainTss",
		"Unique string to identify group of nodes. Share this with your friends to let them connect with you")
	flag.IntVar(&config.TssConfig.Port, "p2p-port", 6668, "listening port local")
	flag.StringVar(&config.TssConfig.ExternalIP, "external-ip", "", "external IP of this node")
	flag.Var(&config.TssConfig.BootstrapPeers, "peer", "Adds a peer multiaddress to the bootstrap list")

	flag.Parse()
	return config
}

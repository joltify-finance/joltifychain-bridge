package config

import (
	"flag"
)

type InvoiceChainConfig struct {
	GrpcAddress string
	WsAddress   string
	WsEndpoint  string
}

type Config struct {
	InvoiceChainConfig InvoiceChainConfig
	KeyringAddress     string
	HomeDir            string
}

func DefaultConfig() Config {
	var config Config
	flag.StringVar(&config.InvoiceChainConfig.GrpcAddress, "grpc-port", "127.0.0.1:9090", "address for invoice chain")
	flag.StringVar(&config.InvoiceChainConfig.WsAddress, "ws-port", "tcp://localhost:26657", "ws address for invoice chain")
	flag.StringVar(&config.InvoiceChainConfig.WsEndpoint, "ws-endpoint", "/websocket", "endpoint for invoice chain")
	flag.StringVar(&config.KeyringAddress, "key", "./keyring.key", "operator key path")
	flag.StringVar(&config.HomeDir, "home", "./", "home director for bridge")
	flag.Parse()
	return config
}

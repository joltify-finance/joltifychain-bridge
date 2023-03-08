package cossubmit

import (
	"context"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	tmclienthttp "github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/simapp/params"
	grpc1 "github.com/gogo/protobuf/grpc"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	bcommon "gitlab.com/joltify/joltifychain-bridge/common"
	"gitlab.com/joltify/joltifychain-bridge/tssclient"
	"google.golang.org/grpc"
)

const (
	grpcTimeout   = time.Second * 30
	submitBackoff = time.Second * 20
	channelSize   = 2000
	capacity      = 10000
)

type CosHandler struct {
	GrpcAddr              string
	httpAddr              string
	Keyring               keyring.Keyring
	logger                zerolog.Logger
	GrpcLock              *sync.RWMutex
	GrpcClient            grpc1.ClientConn
	encoding              *params.EncodingConfig
	tssServer             tssclient.TssInstance
	ChainId               string
	wsClient              *tmclienthttp.HTTP
	retryLock             *sync.Mutex
	currentNewBlockChan   <-chan ctypes.ResultEvent
	CurrentNewValidator   <-chan ctypes.ResultEvent
	ChannelQueueNewBlock  chan ctypes.ResultEvent
	ChannelQueueValidator chan ctypes.ResultEvent
}

func NewCosOperations(grpcAddr, httpAddr string, grpcClient grpc1.ClientConn, keyring keyring.Keyring, logTag string, tssServer tssclient.TssInstance) (*CosHandler, error) {
	var err error
	encode := bcommon.MakeEncodingConfig()
	if grpcClient == nil {
		grpcClient, err = grpc.Dial(grpcAddr, grpc.WithInsecure())
		if err != nil {
			return nil, err
		}
	}

	ts := tmservice.NewServiceClient(grpcClient)
	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	nodeInfo, err := ts.GetNodeInfo(ctx, &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return nil, err
	}

	client, err := tmclienthttp.New(httpAddr, "/websocket")
	if err != nil {
		return nil, err
	}
	err = client.Start()
	if err != nil {
		return nil, err
	}

	handler := &CosHandler{
		GrpcAddr:              grpcAddr,
		httpAddr:              httpAddr,
		Keyring:               keyring,
		logger:                log.With().Str("module", logTag).Logger(),
		tssServer:             tssServer,
		encoding:              &encode,
		GrpcClient:            grpcClient,
		wsClient:              client,
		GrpcLock:              &sync.RWMutex{},
		ChainId:               nodeInfo.DefaultNodeInfo.Network,
		retryLock:             &sync.Mutex{},
		ChannelQueueNewBlock:  make(chan ctypes.ResultEvent, channelSize),
		ChannelQueueValidator: make(chan ctypes.ResultEvent, channelSize),
	}
	return handler, nil
}

func (cs *CosHandler) TerminateTss() {
	cs.tssServer.Stop()
}

func (cs *CosHandler) TerminateCosmosClient() error {
	if cs.wsClient != nil {
		err := cs.wsClient.Stop()
		if err != nil {
			cs.logger.Error().Err(err).Msg("fail to terminate the ws")
		}
	}
	a, ok := cs.GrpcClient.(*grpc.ClientConn)
	// for the test, it is not the grpc.clientconn type, so we skip close it
	if ok {
		return a.Close()
	}
	return nil
}

package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/oppy-finance/oppy-bridge/oppybridge"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// OppyHTTPServer provide http endpoint for tss server
type OppyHTTPServer struct {
	logger     zerolog.Logger
	s          *http.Server
	peerID     string
	ctx        context.Context
	oppyBridge *oppybridge.OppyChainInstance
}

// NewOppyHttpServer should only listen to the loopback
func NewOppyHttpServer(ctx context.Context, tssAddr string, peerID string, oppyBridge *oppybridge.OppyChainInstance) *OppyHTTPServer {
	hs := &OppyHTTPServer{
		logger: log.With().Str("module", "http").Logger(),
		peerID: peerID,
		ctx:    ctx,
	}
	s := &http.Server{
		Addr:              tssAddr,
		Handler:           hs.oppyNewHandler(),
		ReadHeaderTimeout: time.Second * 30,
	}
	hs.s = s
	hs.oppyBridge = oppyBridge
	return hs
}

func logMiddleware() mux.MiddlewareFunc {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Debug().
				Str("route", r.URL.Path).
				Str("port", r.URL.Port()).
				Str("method", r.Method).
				Msg("HTTP request received")

			handler.ServeHTTP(w, r)
		})
	}
}

func (t *OppyHTTPServer) getP2pIDHandler(w http.ResponseWriter, _ *http.Request) {
	_, err := w.Write([]byte(t.peerID))
	if err != nil {
		t.logger.Error().Err(err).Msg("fail to write to response")
	}
}

func (t *OppyHTTPServer) queryPendingTxHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	address := r.URL.Query().Get("address")

	defer func() {
		if err := r.Body.Close(); nil != err {
			t.logger.Error().Err(err).Msg("fail to close request body")
		}
	}()

	_, err := sdk.AccAddressFromBech32(address)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result := t.oppyBridge.QueryPendingTx(address)

	retData, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Add("Access-Control-Allow-Origin", "*")
	_, err = w.Write(retData)
	if err != nil {
		t.logger.Error().Err(err).Msg("fail to write to response")
	}
}

// NewHandler registers the API routes and returns a new HTTP handler
func (t *OppyHTTPServer) oppyNewHandler() http.Handler {
	router := mux.NewRouter()
	router.Handle("/p2pid", http.HandlerFunc(t.getP2pIDHandler)).Methods(http.MethodGet)
	router.Handle("/metrics", promhttp.Handler())
	router.Handle("/pending_tx", http.HandlerFunc(t.queryPendingTxHandler)).Methods(http.MethodGet)
	router.Use(logMiddleware())
	return router
}

func (t *OppyHTTPServer) Start(wg *sync.WaitGroup) error {
	if t.s == nil {
		return errors.New("invalid http server instance")
	}
	var globalErr error
	go func() {
		if err := t.s.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				globalErr = err
			}
		}
	}()

	go func() {
		<-t.ctx.Done()
		err := t.s.Shutdown(t.ctx)
		if err != nil {
			t.logger.Error().Err(err).Msg("fail to shut down the http server gracefully")
		}
		fmt.Printf("we quit the http service")
		wg.Done()
	}()

	return globalErr
}

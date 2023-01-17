package bridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gitlab.com/joltify/joltifychain-bridge/cosbridge"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// CosHTTPServer provide http endpoint for tss server
type CosHTTPServer struct {
	logger    zerolog.Logger
	s         *http.Server
	peerID    string
	ctx       context.Context
	cosBridge *cosbridge.JoltChainInstance
}

// NewCosHttpServer should only listen to the loopback
func NewCosHttpServer(ctx context.Context, tssAddr string, peerID string, oppyBridge *cosbridge.JoltChainInstance) *CosHTTPServer {
	hs := &CosHTTPServer{
		logger: log.With().Str("module", "http").Logger(),
		peerID: peerID,
		ctx:    ctx,
	}
	s := &http.Server{
		Addr:              tssAddr,
		Handler:           hs.bridgeNewHandler(),
		ReadHeaderTimeout: time.Second * 30,
	}
	hs.s = s
	hs.cosBridge = oppyBridge
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

func (t *CosHTTPServer) getP2pIDHandler(w http.ResponseWriter, _ *http.Request) {
	_, err := w.Write([]byte(t.peerID))
	if err != nil {
		t.logger.Error().Err(err).Msg("fail to write to response")
	}
}

// NewHandler registers the API routes and returns a new HTTP handler
func (t *CosHTTPServer) bridgeNewHandler() http.Handler {
	router := mux.NewRouter()
	router.Handle("/p2pid", http.HandlerFunc(t.getP2pIDHandler)).Methods(http.MethodGet)
	router.Handle("/metrics", promhttp.Handler())
	router.Use(logMiddleware())
	return router
}

func (t *CosHTTPServer) Start(wg *sync.WaitGroup) error {
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

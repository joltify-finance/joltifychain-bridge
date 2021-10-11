package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"sync"
)

// TssHttpServer provide http endpoint for tss server
type TssHttpServer struct {
	logger zerolog.Logger
	s      *http.Server
	peerID string
	ctx    context.Context
}

// NewTssHttpServer should only listen to the loopback
func NewTssHttpServer(tssAddr string, peerID string, ctx context.Context) *TssHttpServer {
	hs := &TssHttpServer{
		logger: log.With().Str("module", "http").Logger(),
		peerID: peerID,
		ctx:    ctx,
	}
	s := &http.Server{
		Addr:    tssAddr,
		Handler: hs.tssNewHandler(),
	}
	hs.s = s
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

func (t *TssHttpServer) getP2pIDHandler(w http.ResponseWriter, _ *http.Request) {
	_, err := w.Write([]byte(t.peerID))
	if err != nil {
		t.logger.Error().Err(err).Msg("fail to write to response")
	}
}

// NewHandler registers the API routes and returns a new HTTP handler
func (t *TssHttpServer) tssNewHandler() http.Handler {
	router := mux.NewRouter()
	router.Handle("/p2pid", http.HandlerFunc(t.getP2pIDHandler)).Methods(http.MethodGet)
	router.Handle("/metrics", promhttp.Handler())
	router.Use(logMiddleware())
	return router
}

func (t *TssHttpServer) Start(wg *sync.WaitGroup) error {
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
		t.s.Shutdown(t.ctx)
		fmt.Printf("we quit the http service")
		wg.Done()
	}()

	return globalErr
}

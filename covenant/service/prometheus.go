package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	logger "github.com/rs/zerolog"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// ReadinessChecker defines the interface for checking readiness status
type ReadinessChecker interface {
	CheckReadiness() error
}

type PrometheusServer struct {
	svr *http.Server

	logger *zap.Logger

	interval time.Duration

	quit chan struct{}

	readinessChecker ReadinessChecker
}

func NewPrometheusServer(addr string, interval time.Duration, logger *zap.Logger) *PrometheusServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	return &PrometheusServer{
		svr: &http.Server{
			Handler:           mux,
			Addr:              addr,
			ReadTimeout:       1 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       30 * time.Second,
			ReadHeaderTimeout: 2 * time.Second,
		},
		interval: interval,
		logger:   logger,
		quit:     make(chan struct{}, 1),
	}
}

// SetReadinessChecker sets the readiness checker implementation
func (ps *PrometheusServer) SetReadinessChecker(rc ReadinessChecker) {
	ps.readinessChecker = rc

	// Add the readiness endpoint only after the checker is set
	if mux, ok := ps.svr.Handler.(*http.ServeMux); ok {
		mux.HandleFunc("/health", ps.readinessHandler)
		ps.logger.Info("Readiness endpoint registered at /health")
	}
}

// readinessHandler handles the /health endpoint requests
func (ps *PrometheusServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	if ps.readinessChecker == nil {
		ps.logger.Error("Readiness checker not configured")
		http.Error(w, "Readiness checker not configured", http.StatusInternalServerError)
		return
	}

	if err := ps.readinessChecker.CheckReadiness(); err != nil {
		ps.logger.Error("Readiness check failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Healthy")); err != nil {
		logger.Ctx(r.Context()).Err(err).Msg("failed to write response")
	}
}

func (ps *PrometheusServer) Start() {
	ps.logger.Info("Starting Prometheus server",
		zap.String("address", ps.svr.Addr))

	if err := ps.svr.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			// the Prometheus server is shutdown
			return
		}
		ps.logger.Fatal("failed to start Prometheus server",
			zap.Error(err))
	}
}

func (ps *PrometheusServer) Stop() {
	ps.logger.Info("Stopping Prometheus server")

	close(ps.quit)
	if err := ps.svr.Shutdown(context.Background()); err != nil {
		ps.logger.Error("failed to stop the Prometheus server",
			zap.Error(err))
		ps.logger.Info("force stopping the Prometheus server")
		if err = ps.svr.Close(); err != nil {
			ps.logger.Error("failed to force stopping the Prometheus server",
				zap.Error(err))
		}
	}
}

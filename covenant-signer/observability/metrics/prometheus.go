package metrics

import (
	"net/http"
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

const (
	metricRequestTimeout     time.Duration = 15 * time.Second
	metricRequestIdleTimeout time.Duration = 30 * time.Second
)

func Start(addr string, reg *prometheus.Registry) {
	go start(addr, reg)
}

func start(addr string, reg *prometheus.Registry) {
	// Add Go module build info.
	reg.MustRegister(collectors.NewBuildInfoCollector())
	reg.MustRegister(collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.GoRuntimeMetricsRule{Matcher: regexp.MustCompile("/.*")})),
	)

	mux := http.NewServeMux()
	// Expose the registered metrics via HTTP.
	mux.Handle("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  metricRequestTimeout,
		WriteTimeout: metricRequestTimeout,
		IdleTimeout:  metricRequestIdleTimeout,
	}

	log.Printf("Starting metrics server on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msgf("Error starting metrics server on %s", addr)
	}
}

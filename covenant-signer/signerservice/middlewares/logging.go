package middlewares

import (
	"net/http"
	"time"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/observability/tracing"

	"github.com/rs/zerolog/log"
)

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		logger := log.With().Str("path", r.URL.Path).Logger()

		// Attach traceId into each log within the request chain
		traceId := r.Context().Value(tracing.TraceIdKey)
		if traceId != nil {
			logger = logger.With().Interface("traceId", traceId).Logger()
		}

		logger.Debug().Msg("request received")
		r = r.WithContext(logger.WithContext(r.Context()))

		next.ServeHTTP(w, r)

		requestDuration := time.Since(startTime).Milliseconds()
		logEvent := logger.Info()

		tracingInfo := r.Context().Value(tracing.TraceInfoKey)
		if tracingInfo != nil {
			logEvent = logEvent.Interface("tracingInfo", tracingInfo)
		}

		logEvent.Interface("requestDuration", requestDuration).Msg("Request completed")
	})
}

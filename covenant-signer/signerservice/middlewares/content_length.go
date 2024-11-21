package middlewares

import (
	"net/http"
)

func ContentLengthMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > int64(maxBytes) {
				http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			h.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
	return f
}

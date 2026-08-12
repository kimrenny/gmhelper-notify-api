package middleware

import (
	"net/http"
	"time"

	"github.com/gmhelper/notify-api/internal/infra/logger"
)

func Logging(log logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			log.Info("http request",
				logger.String("method", r.Method),
				logger.String("path", r.URL.Path),
				logger.String("remote_addr", r.RemoteAddr),
				logger.String("duration", time.Since(start).String()),
			)
		})
	}
}

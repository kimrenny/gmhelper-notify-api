package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type principalKeyType struct{}

var principalKey = principalKeyType{}

func GetPrincipal(ctx context.Context) (*domain.Principal, bool) {
	p, ok := ctx.Value(principalKey).(*domain.Principal)
	return p, ok && p != nil
}

func ContextWithPrincipal(ctx context.Context, p *domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func Authenticate(verifier auth.TokenVerifier, log logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if authHeader == "" {
				response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid authorization header format, expected 'Bearer <token>'")
				return
			}

			tokenString := strings.TrimSpace(parts[1])
			if tokenString == "" {
				response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "empty bearer token")
				return
			}

			principal, err := verifier.Verify(tokenString)
			if err != nil {
				if log != nil {
					log.Warn("authentication verification failed", logger.Error(err))
				}
				response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
				return
			}

			ctx := ContextWithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

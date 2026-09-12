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

// DefaultAdminRoles lists the standard administrative roles in the GMHelper ecosystem.
var DefaultAdminRoles = []string{"admin", "owner", "service"}

// RequireRole checks if the authenticated principal has one of the specified allowed roles.
// Returns 401 Unauthorized if no principal is present in the request context.
// Returns 403 Forbidden if the principal's role is not within allowedRoles.
func RequireRole(allowedRoles ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := GetPrincipal(r.Context())
			if !ok || principal == nil {
				response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			userRole := strings.TrimSpace(principal.Role)
			for _, allowed := range allowedRoles {
				if strings.EqualFold(userRole, strings.TrimSpace(allowed)) {
					next.ServeHTTP(w, r)
					return
				}
			}

			response.Error(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
		})
	}
}

// RequireAdminRole restricts access to principals with administrative roles (admin, owner, service).
func RequireAdminRole() Middleware {
	return RequireRole(DefaultAdminRoles...)
}

// AdminAuth combines authentication via JWT and role-based authorization for administrative access.
func AdminAuth(verifier auth.TokenVerifier, log logger.Logger) Middleware {
	return Combine(
		Authenticate(verifier, log),
		RequireAdminRole(),
	)
}

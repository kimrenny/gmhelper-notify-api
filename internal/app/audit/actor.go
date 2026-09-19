package audit

import (
	"context"
	"errors"
	"strings"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
)

var (
	ErrInvalidActor     = errors.New("invalid audit actor")
	ErrMissingPrincipal = errors.New("missing or unauthenticated principal in context")
)

// Actor represents the identity responsible for triggering an audit event.
type Actor struct {
	Type   domain.ActivityActorType
	UserID *string
	Name   *string
	Role   *string
}

// UserActor constructs a user actor with the specified user ID and optional display name and role.
func UserActor(userID string, name *string, role *string) Actor {
	trimmedID := strings.TrimSpace(userID)
	var uid *string
	if trimmedID != "" {
		uid = &trimmedID
	}
	return Actor{
		Type:   domain.ActorTypeUser,
		UserID: uid,
		Name:   cleanOptionalString(name),
		Role:   cleanOptionalString(role),
	}
}

// UserActorFromPrincipal constructs a user actor from domain.Principal with an optional display name.
// Returns ErrMissingPrincipal if principal is nil or has an empty user ID.
func UserActorFromPrincipal(p *domain.Principal, name *string) (Actor, error) {
	if p == nil || strings.TrimSpace(p.UserID) == "" {
		return Actor{}, ErrMissingPrincipal
	}
	trimmedID := strings.TrimSpace(p.UserID)
	var role *string
	if r := strings.TrimSpace(p.Role); r != "" {
		role = &r
	}
	return Actor{
		Type:   domain.ActorTypeUser,
		UserID: &trimmedID,
		Name:   cleanOptionalString(name),
		Role:   role,
	}, nil
}

// SystemActor constructs an automated system or background worker actor.
func SystemActor() Actor {
	return Actor{
		Type:   domain.ActorTypeSystem,
		UserID: nil,
		Name:   nil,
		Role:   nil,
	}
}

// ServiceActor constructs an external or service-to-service caller actor.
func ServiceActor(serviceName *string) Actor {
	return Actor{
		Type:   domain.ActorTypeService,
		UserID: nil,
		Name:   cleanOptionalString(serviceName),
		Role:   nil,
	}
}

// ActorFromContext extracts the authenticated user principal from context.
// Returns ErrMissingPrincipal if no valid authenticated principal exists in context.
// Does NOT silently fall back to a system actor.
func ActorFromContext(ctx context.Context, name *string) (Actor, error) {
	if ctx == nil {
		return Actor{}, ErrMissingPrincipal
	}
	principal, ok := middleware.GetPrincipal(ctx)
	if !ok || principal == nil || strings.TrimSpace(principal.UserID) == "" {
		return Actor{}, ErrMissingPrincipal
	}
	return UserActorFromPrincipal(principal, name)
}

// Validate verifies that actor fields are consistent with its actor type.
func (a Actor) Validate() error {
	if !a.Type.IsValid() {
		return ErrInvalidActor
	}
	switch a.Type {
	case domain.ActorTypeUser:
		if a.UserID == nil || strings.TrimSpace(*a.UserID) == "" {
			return ErrInvalidActor
		}
	case domain.ActorTypeSystem, domain.ActorTypeService:
		if a.UserID != nil {
			return ErrInvalidActor
		}
	}
	return nil
}

func cleanOptionalString(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
)

var (
	ErrInvalidInput   = errors.New("invalid audit input")
	ErrInvalidDetails = errors.New("invalid audit details")
	ErrNotFound       = domain.ErrNotFound
)

// RecordInput defines parameters for creating an audit activity log entry.
type RecordInput struct {
	EventType    string
	Actor        Actor
	TargetType   domain.ActivityTargetType
	TargetID     string
	TargetName   *string
	Status       domain.ActivityStatus
	Summary      string
	Details      any
	ErrorMessage *string
	CreatedAt    time.Time
}

// Service provides high-level business audit logging operations.
type Service struct {
	repo domain.ActivityLogRepository
}

// NewService constructs a new AuditService instance.
func NewService(repo domain.ActivityLogRepository) *Service {
	return &Service{repo: repo}
}

// Record creates and persists an immutable audit log entry.
func (s *Service) Record(ctx context.Context, input RecordInput) (*domain.ActivityLog, error) {
	if s.repo == nil {
		return nil, errors.New("activity log repository is not initialized")
	}

	trimmedEventType := strings.TrimSpace(input.EventType)
	if trimmedEventType == "" {
		return nil, fmt.Errorf("%w: event type cannot be empty", ErrInvalidInput)
	}

	if err := input.Actor.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	if !input.TargetType.IsValid() {
		return nil, fmt.Errorf("%w: invalid target type %q", ErrInvalidInput, input.TargetType)
	}

	trimmedTargetID := strings.TrimSpace(input.TargetID)
	if trimmedTargetID == "" {
		return nil, fmt.Errorf("%w: target ID cannot be empty", ErrInvalidInput)
	}

	trimmedSummary := strings.TrimSpace(input.Summary)
	if trimmedSummary == "" {
		return nil, fmt.Errorf("%w: summary cannot be empty", ErrInvalidInput)
	}

	status := input.Status
	if status == "" {
		status = domain.ActivityStatusSuccess
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", ErrInvalidInput, status)
	}

	rawDetails, err := serializeDetails(input.Details)
	if err != nil {
		return nil, err
	}

	log := &domain.ActivityLog{
		EventType:    trimmedEventType,
		ActorType:    input.Actor.Type,
		ActorUserID:  input.Actor.UserID,
		ActorName:    cleanOptionalString(input.Actor.Name),
		ActorRole:    cleanOptionalString(input.Actor.Role),
		TargetType:   input.TargetType,
		TargetID:     trimmedTargetID,
		TargetName:   cleanOptionalString(input.TargetName),
		Status:       status,
		Summary:      trimmedSummary,
		Details:      rawDetails,
		ErrorMessage: cleanOptionalString(input.ErrorMessage),
		CreatedAt:    input.CreatedAt,
	}

	if err := s.repo.Create(ctx, log); err != nil {
		return nil, err
	}

	return log, nil
}

// GetByID fetches a single activity log by its unique identifier.
func (s *Service) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil, fmt.Errorf("%w: id cannot be empty", ErrInvalidInput)
	}
	return s.repo.GetByID(ctx, trimmedID)
}

// List returns paginated activity logs and the total count.
func (s *Service) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	return s.repo.List(ctx, filter)
}

// serializeDetails safely serializes arbitrary structured details into json.RawMessage.
func serializeDetails(details any) (json.RawMessage, error) {
	if details == nil {
		return json.RawMessage("{}"), nil
	}

	switch v := details.(type) {
	case json.RawMessage:
		if len(v) == 0 {
			return json.RawMessage("{}"), nil
		}
		var js any
		if err := json.Unmarshal(v, &js); err != nil {
			return nil, fmt.Errorf("%w: invalid JSON raw message: %v", ErrInvalidDetails, err)
		}
		return v, nil

	case []byte:
		if len(v) == 0 {
			return json.RawMessage("{}"), nil
		}
		var js any
		if err := json.Unmarshal(v, &js); err != nil {
			return nil, fmt.Errorf("%w: invalid JSON bytes: %v", ErrInvalidDetails, err)
		}
		return json.RawMessage(v), nil

	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return json.RawMessage("{}"), nil
		}
		var js any
		if err := json.Unmarshal([]byte(trimmed), &js); err != nil {
			return nil, fmt.Errorf("%w: string is not valid JSON: %v", ErrInvalidDetails, err)
		}
		return json.RawMessage(trimmed), nil

	default:
		bytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to marshal details to JSON: %v", ErrInvalidDetails, err)
		}
		return json.RawMessage(bytes), nil
	}
}

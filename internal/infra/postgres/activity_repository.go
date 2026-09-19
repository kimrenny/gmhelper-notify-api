package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ActivityLogRepository implements domain.ActivityLogRepository for PostgreSQL.
// This repository is append-only and provides no update or delete operations to preserve audit immutability.
type ActivityLogRepository struct {
	db *sql.DB
}

// NewActivityLogRepository creates a new instance of ActivityLogRepository.
func NewActivityLogRepository(db *sql.DB) *ActivityLogRepository {
	return &ActivityLogRepository{db: db}
}

// Create inserts a new activity log record into the database.
func (r *ActivityLogRepository) Create(ctx context.Context, log *domain.ActivityLog) error {
	if log == nil {
		return domain.ErrInvalidEntity
	}
	if log.ID == "" {
		log.ID = uuid.New().String()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if err := log.Validate(ctx); err != nil {
		return err
	}

	details := []byte("{}")
	if len(log.Details) > 0 {
		details = log.Details
	}

	query := `
INSERT INTO activity_logs (
	id, event_type, actor_type, actor_user_id, actor_name, actor_role,
	target_type, target_id, target_name, status, summary, details, error_message, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err := r.db.ExecContext(ctx, query,
		log.ID,
		log.EventType,
		log.ActorType,
		log.ActorUserID,
		log.ActorName,
		log.ActorRole,
		log.TargetType,
		log.TargetID,
		log.TargetName,
		log.Status,
		log.Summary,
		details,
		log.ErrorMessage,
		log.CreatedAt,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return domain.ErrConflict
		}
		return err
	}

	return nil
}

// GetByID retrieves a single activity log record by its unique ID.
func (r *ActivityLogRepository) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	query := `
SELECT id, event_type, actor_type, actor_user_id, actor_name, actor_role,
       target_type, target_id, target_name, status, summary, details, error_message, created_at
FROM activity_logs
WHERE id = $1`

	row := r.db.QueryRowContext(ctx, query, id)

	log := &domain.ActivityLog{}
	var rawDetails []byte

	err := row.Scan(
		&log.ID,
		&log.EventType,
		&log.ActorType,
		&log.ActorUserID,
		&log.ActorName,
		&log.ActorRole,
		&log.TargetType,
		&log.TargetID,
		&log.TargetName,
		&log.Status,
		&log.Summary,
		&rawDetails,
		&log.ErrorMessage,
		&log.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	if len(rawDetails) > 0 {
		log.Details = json.RawMessage(rawDetails)
	} else {
		log.Details = json.RawMessage(`{}`)
	}

	return log, nil
}

// List retrieves paginated activity logs ordered newest first (created_at DESC, id DESC), returning total count.
func (r *ActivityLogRepository) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var whereClauses []string
	var args []any
	argIdx := 1

	if filter.EventType != nil && strings.TrimSpace(*filter.EventType) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("event_type = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.EventType))
		argIdx++
	}

	if filter.ActorUserID != nil && strings.TrimSpace(*filter.ActorUserID) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("actor_user_id = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.ActorUserID))
		argIdx++
	}

	if filter.TargetType != nil && strings.TrimSpace(string(*filter.TargetType)) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("target_type = $%d", argIdx))
		args = append(args, string(*filter.TargetType))
		argIdx++
	}

	if filter.TargetID != nil && strings.TrimSpace(*filter.TargetID) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("target_id = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.TargetID))
		argIdx++
	}

	if filter.Status != nil && strings.TrimSpace(string(*filter.Status)) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*filter.Status))
		argIdx++
	}

	if filter.FromDate != nil && !filter.FromDate.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, filter.FromDate.UTC())
		argIdx++
	}

	if filter.ToDate != nil && !filter.ToDate.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, filter.ToDate.UTC())
		argIdx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// 1. Total count
	var total int
	countQuery := "SELECT COUNT(*) FROM activity_logs" + whereSQL
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return []*domain.ActivityLog{}, 0, nil
	}

	// 2. Paginated rows ordered by created_at DESC, id DESC
	query := fmt.Sprintf(`
SELECT id, event_type, actor_type, actor_user_id, actor_name, actor_role,
       target_type, target_id, target_name, status, summary, details, error_message, created_at
FROM activity_logs%s
ORDER BY created_at DESC, id DESC
LIMIT $%d OFFSET $%d`, whereSQL, argIdx, argIdx+1)

	queryArgs := append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]*domain.ActivityLog, 0, limit)
	for rows.Next() {
		log := &domain.ActivityLog{}
		var rawDetails []byte

		if err := rows.Scan(
			&log.ID,
			&log.EventType,
			&log.ActorType,
			&log.ActorUserID,
			&log.ActorName,
			&log.ActorRole,
			&log.TargetType,
			&log.TargetID,
			&log.TargetName,
			&log.Status,
			&log.Summary,
			&rawDetails,
			&log.ErrorMessage,
			&log.CreatedAt,
		); err != nil {
			return nil, 0, err
		}

		if len(rawDetails) > 0 {
			log.Details = json.RawMessage(rawDetails)
		} else {
			log.Details = json.RawMessage(`{}`)
		}

		items = append(items, log)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

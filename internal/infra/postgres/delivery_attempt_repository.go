package postgres

import (
	"context"
	"database/sql"

	"github.com/gmhelper/notify-api/internal/domain"
)

type DeliveryAttemptRepository struct {
	db *sql.DB
}

func NewDeliveryAttemptRepository(db *sql.DB) *DeliveryAttemptRepository {
	return &DeliveryAttemptRepository{db: db}
}

func (r *DeliveryAttemptRepository) Create(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	if !attempt.TargetType.IsValid() || !attempt.Status.IsValid() || attempt.ID == "" || attempt.TargetID == "" || attempt.AttemptNumber <= 0 {
		return domain.ErrInvalidEntity
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO delivery_attempts (id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, attempt.ID, attempt.TargetType, attempt.TargetID, attempt.Status, attempt.AttemptNumber, attempt.ErrorMessage, attempt.AttemptedAt, attempt.CreatedAt)
	return err
}

func (r *DeliveryAttemptRepository) ListByTarget(ctx context.Context, targetType domain.DeliveryTargetType, targetID string) ([]*domain.DeliveryAttempt, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, target_type, target_id, status, attempt_number, error_message, attempted_at, created_at
FROM delivery_attempts
WHERE target_type = $1 AND target_id = $2`, targetType, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attempts := []*domain.DeliveryAttempt{}
	for rows.Next() {
		attempt := &domain.DeliveryAttempt{}
		if err := rows.Scan(&attempt.ID, &attempt.TargetType, &attempt.TargetID, &attempt.Status, &attempt.AttemptNumber, &attempt.ErrorMessage, &attempt.AttemptedAt, &attempt.CreatedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

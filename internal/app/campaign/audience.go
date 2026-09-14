package campaign

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
	"github.com/google/uuid"
)

const defaultAudiencePageSize = 250

// UserLister defines the client contract for retrieving paginated active/unblocked users.
type UserLister interface {
	GetUsers(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error)
}

// AudiencePopulator defines the interface for populating campaign recipients for a claimed campaign.
type AudiencePopulator interface {
	PopulateAudience(ctx context.Context, campaignID string) (int, error)
}

// Populator handles resolving users and writing recipient rows for a running campaign.
type Populator struct {
	userLister    UserLister
	recipientRepo domain.CampaignRecipientRepository
	campaignRepo  domain.NotificationCampaignRepository
	logger        logger.Logger
	pageSize      int
}

// NewAudiencePopulator constructs an AudiencePopulator.
func NewAudiencePopulator(
	userLister UserLister,
	recipientRepo domain.CampaignRecipientRepository,
	campaignRepo domain.NotificationCampaignRepository,
	logger logger.Logger,
) *Populator {
	return &Populator{
		userLister:    userLister,
		recipientRepo: recipientRepo,
		campaignRepo:  campaignRepo,
		logger:        logger,
		pageSize:      defaultAudiencePageSize,
	}
}

// PopulateAudience fetches users in bounded pages from gmhelper-api and inserts CampaignRecipient records.
// On any retrieval or insertion failure, the campaign is transitioned to failed status.
func (p *Populator) PopulateAudience(ctx context.Context, campaignID string) (int, error) {
	if p.userLister == nil || p.recipientRepo == nil || p.campaignRepo == nil {
		return 0, errors.New("populator dependencies not configured")
	}

	page := 1
	totalInserted := 0
	now := time.Now().UTC()

	for {
		if ctx.Err() != nil {
			return totalInserted, ctx.Err()
		}

		paged, err := p.userLister.GetUsers(ctx, page, p.pageSize, true, true)
		if err != nil {
			if p.logger != nil {
				p.logger.Error("failed to retrieve user page for campaign audience",
					logger.String("campaignId", campaignID),
					logger.Int("page", page),
					logger.Error(err),
				)
			}
			completedAt := time.Now().UTC()
			_ = p.campaignRepo.UpdateStatus(ctx, campaignID, domain.CampaignStatusFailed, nil, &completedAt)
			return totalInserted, err
		}

		for _, u := range paged.Items {
			trimmedEmail := strings.TrimSpace(u.Email)
			if trimmedEmail == "" {
				continue
			}

			recipient := &domain.CampaignRecipient{
				ID:             uuid.NewString(),
				CampaignID:     campaignID,
				ExternalUserID: strings.TrimSpace(u.ID),
				RecipientEmail: trimmedEmail,
				RecipientName:  strings.TrimSpace(u.Username),
				DeliveryStatus: domain.DeliveryStatusPending,
				AttemptsCount:  0,
				CreatedAt:      now,
				UpdatedAt:      now,
			}

			if err := p.recipientRepo.Create(ctx, recipient); err != nil {
				if p.logger != nil {
					p.logger.Error("failed to insert campaign recipient",
						logger.String("campaignId", campaignID),
						logger.String("userId", u.ID),
						logger.Error(err),
					)
				}
				completedAt := time.Now().UTC()
				_ = p.campaignRepo.UpdateStatus(ctx, campaignID, domain.CampaignStatusFailed, nil, &completedAt)
				return totalInserted, err
			}
			totalInserted++
		}

		if !paged.HasNextPage || len(paged.Items) == 0 {
			break
		}
		page++
	}

	if p.logger != nil {
		p.logger.Info("successfully populated campaign audience",
			logger.String("campaignId", campaignID),
			logger.Int("recipientsCount", totalInserted),
		)
	}

	return totalInserted, nil
}

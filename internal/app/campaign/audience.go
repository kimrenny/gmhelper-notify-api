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
	GetUsersFiltered(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool, filter *domain.CampaignAudienceFilter) (*userclient.PagedUsers, error)
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
		if p.logger != nil {
			p.logger.Error("populator dependencies not configured", logger.String("campaignId", campaignID))
		}
		return 0, errors.New("populator dependencies not configured")
	}

	var audienceFilter *domain.CampaignAudienceFilter
	if camp, err := p.campaignRepo.GetByID(ctx, campaignID); err == nil && camp != nil {
		audienceFilter = camp.AudienceFilter
	}

	if p.logger != nil {
		p.logger.Info("audience population started",
			logger.String("campaignId", campaignID),
			logger.Int("pageSize", p.pageSize),
		)
	}

	page := 1
	totalInserted := 0
	now := time.Now().UTC()

	for {
		if ctx.Err() != nil {
			if p.logger != nil {
				p.logger.Warn("audience population cancelled by context",
					logger.String("campaignId", campaignID),
					logger.Error(ctx.Err()),
				)
			}
			return totalInserted, ctx.Err()
		}

		if p.logger != nil {
			p.logger.Debug("requesting users page for campaign audience",
				logger.String("campaignId", campaignID),
				logger.Int("page", page),
			)
		}

		paged, err := p.userLister.GetUsersFiltered(ctx, page, p.pageSize, true, true, audienceFilter)
		if err != nil {
			if p.logger != nil {
				p.logger.Error("audience population failed to retrieve user page",
					logger.String("campaignId", campaignID),
					logger.Int("page", page),
					logger.Error(err),
				)
			}
			completedAt := time.Now().UTC()
			_ = p.campaignRepo.UpdateStatus(ctx, campaignID, domain.CampaignStatusFailed, nil, &completedAt)
			return totalInserted, err
		}

		if p.logger != nil {
			p.logger.Info("users page received for campaign audience",
				logger.String("campaignId", campaignID),
				logger.Int("page", page),
				logger.Int("itemsCount", len(paged.Items)),
				logger.Int("totalCount", paged.TotalCount),
				logger.Bool("hasNextPage", paged.HasNextPage),
			)
		}

		pageInserted := 0
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
			pageInserted++
			totalInserted++
		}

		if p.logger != nil {
			p.logger.Info("recipients created for page",
				logger.String("campaignId", campaignID),
				logger.Int("page", page),
				logger.Int("pageInserted", pageInserted),
				logger.Int("totalInserted", totalInserted),
			)
		}

		if !paged.HasNextPage || len(paged.Items) == 0 {
			break
		}
		page++
	}

	if totalInserted == 0 {
		if p.logger != nil {
			p.logger.Warn("campaign audience resolved to zero eligible recipients; marking campaign failed",
				logger.String("campaignId", campaignID),
			)
		}
		completedAt := time.Now().UTC()
		_ = p.campaignRepo.UpdateStatus(ctx, campaignID, domain.CampaignStatusFailed, nil, &completedAt)
	} else if p.logger != nil {
		p.logger.Info("audience population completed successfully",
			logger.String("campaignId", campaignID),
			logger.Int("recipientsCount", totalInserted),
		)
	}

	return totalInserted, nil
}

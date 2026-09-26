package campaign

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

type mockUserLister struct {
	getUsersFunc         func(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error)
	getUsersFilteredFunc func(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool, filter *domain.CampaignAudienceFilter) (*userclient.PagedUsers, error)
	mu                   sync.Mutex
	calls                []int
}

func (m *mockUserLister) GetUsers(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error) {
	return m.GetUsersFiltered(ctx, page, pageSize, activeOnly, unblockedOnly, nil)
}

func (m *mockUserLister) GetUsersFiltered(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool, filter *domain.CampaignAudienceFilter) (*userclient.PagedUsers, error) {
	m.mu.Lock()
	m.calls = append(m.calls, page)
	m.mu.Unlock()

	if m.getUsersFilteredFunc != nil {
		return m.getUsersFilteredFunc(ctx, page, pageSize, activeOnly, unblockedOnly, filter)
	}
	if m.getUsersFunc != nil {
		return m.getUsersFunc(ctx, page, pageSize, activeOnly, unblockedOnly)
	}
	return &userclient.PagedUsers{}, nil
}

type mockRecipientRepo struct {
	mu         sync.Mutex
	recipients map[string]*domain.CampaignRecipient
	createFunc func(ctx context.Context, recipient *domain.CampaignRecipient) error
}

func newMockRecipientRepo() *mockRecipientRepo {
	return &mockRecipientRepo{
		recipients: make(map[string]*domain.CampaignRecipient),
	}
}

func (m *mockRecipientRepo) GetByID(ctx context.Context, id string) (*domain.CampaignRecipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recipients[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r, nil
}

func (m *mockRecipientRepo) Create(ctx context.Context, recipient *domain.CampaignRecipient) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createFunc != nil {
		return m.createFunc(ctx, recipient)
	}
	m.recipients[recipient.ID] = recipient
	return nil
}

func (m *mockRecipientRepo) ListByCampaignID(ctx context.Context, campaignID string) ([]*domain.CampaignRecipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*domain.CampaignRecipient
	for _, r := range m.recipients {
		if r.CampaignID == campaignID {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockRecipientRepo) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recipients[id]
	if !ok {
		return domain.ErrNotFound
	}
	r.DeliveryStatus = status
	r.AttemptsCount = attempts
	r.LastAttemptAt = lastAttemptAt
	r.SentAt = sentAt
	r.ErrorMessage = errorMessage
	return nil
}

func (m *mockRecipientRepo) ClaimPending(ctx context.Context, limit int) ([]*domain.CampaignRecipient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var claimed []*domain.CampaignRecipient
	now := time.Now().UTC()
	for _, r := range m.recipients {
		if r.DeliveryStatus == domain.DeliveryStatusPending {
			r.DeliveryStatus = domain.DeliveryStatusSending
			r.AttemptsCount++
			r.LastAttemptAt = &now
			claimed = append(claimed, r)
			if len(claimed) >= limit {
				break
			}
		}
	}
	return claimed, nil
}

func (m *mockRecipientRepo) RecoverStaleSending(ctx context.Context, olderThan time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	cutoff := time.Now().UTC().Add(-olderThan)
	for _, r := range m.recipients {
		if r.DeliveryStatus == domain.DeliveryStatusSending && r.LastAttemptAt != nil && r.LastAttemptAt.Before(cutoff) {
			r.DeliveryStatus = domain.DeliveryStatusPending
			count++
		}
	}
	return count, nil
}

func (m *mockRecipientRepo) GetDeliveryStatsByCampaign(ctx context.Context, campaignID string) (*domain.CampaignDeliveryStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := &domain.CampaignDeliveryStats{}
	for _, r := range m.recipients {
		if r.CampaignID == campaignID {
			stats.TotalCount++
			switch r.DeliveryStatus {
			case domain.DeliveryStatusPending:
				stats.PendingCount++
			case domain.DeliveryStatusSending:
				stats.SendingCount++
			case domain.DeliveryStatusSent:
				stats.SentCount++
			case domain.DeliveryStatusFailed:
				stats.FailedCount++
			}
		}
	}
	return stats, nil
}

func TestAudiencePopulator_MultiPageSuccess(t *testing.T) {
	campaignID := "c-test-1"
	lister := &mockUserLister{
		getUsersFunc: func(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error) {
			if !activeOnly || !unblockedOnly {
				t.Errorf("expected activeOnly=true, unblockedOnly=true")
			}
			if page == 1 {
				return &userclient.PagedUsers{
					Items: []userclient.User{
						{ID: "u-1", Username: "user1", Email: "u1@example.com"},
						{ID: "u-2", Username: "user2", Email: "u2@example.com"},
					},
					TotalCount:  3,
					Page:        1,
					PageSize:    2,
					HasNextPage: true,
				}, nil
			}
			if page == 2 {
				return &userclient.PagedUsers{
					Items: []userclient.User{
						{ID: "u-3", Username: "user3", Email: "u3@example.com"},
					},
					TotalCount:  3,
					Page:        2,
					PageSize:    2,
					HasNextPage: false,
				}, nil
			}
			return &userclient.PagedUsers{}, nil
		},
	}

	recRepo := newMockRecipientRepo()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			campaignID: {ID: campaignID, Status: domain.CampaignStatusRunning},
		},
	}
	log, _ := logger.NewLogger("error")

	populator := NewAudiencePopulator(lister, recRepo, cRepo, log)
	count, err := populator.PopulateAudience(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 recipients inserted, got %d", count)
	}

	if len(recRepo.recipients) != 3 {
		t.Fatalf("expected 3 recipients in repo, got %d", len(recRepo.recipients))
	}

	// Verify fields
	for _, r := range recRepo.recipients {
		if r.CampaignID != campaignID {
			t.Errorf("expected campaign ID %s, got %s", campaignID, r.CampaignID)
		}
		if r.DeliveryStatus != domain.DeliveryStatusPending {
			t.Errorf("expected status pending, got %s", r.DeliveryStatus)
		}
		if r.AttemptsCount != 0 {
			t.Errorf("expected attemptsCount 0, got %d", r.AttemptsCount)
		}
	}
}

func TestAudiencePopulator_SkipUsersWithoutEmail(t *testing.T) {
	campaignID := "c-test-2"
	lister := &mockUserLister{
		getUsersFunc: func(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error) {
			return &userclient.PagedUsers{
				Items: []userclient.User{
					{ID: "u-1", Username: "user1", Email: ""},
					{ID: "u-2", Username: "user2", Email: "   "},
					{ID: "u-3", Username: "user3", Email: "u3@example.com"},
				},
				TotalCount:  3,
				Page:        1,
				PageSize:    250,
				HasNextPage: false,
			}, nil
		},
	}

	recRepo := newMockRecipientRepo()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			campaignID: {ID: campaignID, Status: domain.CampaignStatusRunning},
		},
	}
	log, _ := logger.NewLogger("error")

	populator := NewAudiencePopulator(lister, recRepo, cRepo, log)
	count, err := populator.PopulateAudience(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 valid recipient inserted, got %d", count)
	}

	for _, r := range recRepo.recipients {
		if r.ExternalUserID != "u-3" || r.RecipientEmail != "u3@example.com" {
			t.Errorf("unexpected recipient: %+v", r)
		}
	}
}

func TestAudiencePopulator_EmptyAudience(t *testing.T) {
	campaignID := "c-test-empty"
	lister := &mockUserLister{
		getUsersFunc: func(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error) {
			return &userclient.PagedUsers{
				Items:       []userclient.User{},
				TotalCount:  0,
				Page:        1,
				PageSize:    250,
				HasNextPage: false,
			}, nil
		},
	}

	recRepo := newMockRecipientRepo()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			campaignID: {ID: campaignID, Status: domain.CampaignStatusRunning},
		},
	}
	log, _ := logger.NewLogger("error")

	populator := NewAudiencePopulator(lister, recRepo, cRepo, log)
	count, err := populator.PopulateAudience(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected 0 recipients, got %d", count)
	}

	if cRepo.campaigns[campaignID].Status != domain.CampaignStatusFailed {
		t.Fatalf("expected campaign status failed for empty audience, got %s", cRepo.campaigns[campaignID].Status)
	}
}

func TestAudiencePopulator_UserListerError_MarksCampaignFailed(t *testing.T) {
	campaignID := "c-test-err"
	lister := &mockUserLister{
		getUsersFunc: func(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error) {
			return nil, errors.New("network timeout")
		},
	}

	recRepo := newMockRecipientRepo()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			campaignID: {ID: campaignID, Status: domain.CampaignStatusRunning},
		},
	}
	log, _ := logger.NewLogger("error")

	populator := NewAudiencePopulator(lister, recRepo, cRepo, log)
	_, err := populator.PopulateAudience(context.Background(), campaignID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if cRepo.campaigns[campaignID].Status != domain.CampaignStatusFailed {
		t.Fatalf("expected campaign status failed, got %s", cRepo.campaigns[campaignID].Status)
	}
}

func TestAudiencePopulator_RecipientRepoError_MarksCampaignFailed(t *testing.T) {
	campaignID := "c-test-dberr"
	lister := &mockUserLister{
		getUsersFunc: func(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool) (*userclient.PagedUsers, error) {
			return &userclient.PagedUsers{
				Items: []userclient.User{
					{ID: "u-1", Username: "user1", Email: "u1@example.com"},
				},
				HasNextPage: false,
			}, nil
		},
	}

	recRepo := newMockRecipientRepo()
	recRepo.createFunc = func(ctx context.Context, recipient *domain.CampaignRecipient) error {
		return errors.New("disk full")
	}

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			campaignID: {ID: campaignID, Status: domain.CampaignStatusRunning},
		},
	}
	log, _ := logger.NewLogger("error")

	populator := NewAudiencePopulator(lister, recRepo, cRepo, log)
	_, err := populator.PopulateAudience(context.Background(), campaignID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if cRepo.campaigns[campaignID].Status != domain.CampaignStatusFailed {
		t.Fatalf("expected campaign status failed, got %s", cRepo.campaigns[campaignID].Status)
	}
}

func TestAudiencePopulator_WithCampaignAudienceFilter(t *testing.T) {
	campaignID := "c-test-filtered"
	filter := &domain.CampaignAudienceFilter{
		Role:             "Admin",
		Language:         "EN",
		RegistrationDate: "last_7_days",
		EmailConfirmed:   "confirmed",
		AccountStatus:    "active",
	}

	var capturedFilter *domain.CampaignAudienceFilter
	lister := &mockUserLister{
		getUsersFilteredFunc: func(ctx context.Context, page, pageSize int, activeOnly, unblockedOnly bool, f *domain.CampaignAudienceFilter) (*userclient.PagedUsers, error) {
			capturedFilter = f
			return &userclient.PagedUsers{
				Items: []userclient.User{
					{ID: "u-admin", Username: "admin1", Email: "admin1@example.com", Role: "Admin", Language: "EN"},
				},
				TotalCount:  1,
				Page:        1,
				PageSize:    250,
				HasNextPage: false,
			}, nil
		},
	}

	recRepo := newMockRecipientRepo()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			campaignID: {
				ID:             campaignID,
				Status:         domain.CampaignStatusRunning,
				AudienceFilter: filter,
			},
		},
	}
	log, _ := logger.NewLogger("error")

	populator := NewAudiencePopulator(lister, recRepo, cRepo, log)
	count, err := populator.PopulateAudience(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 recipient, got %d", count)
	}

	if capturedFilter == nil {
		t.Fatal("expected audience filter to be passed to userclient, got nil")
	}

	if capturedFilter.Role != "Admin" || capturedFilter.Language != "EN" || capturedFilter.RegistrationDate != "last_7_days" {
		t.Errorf("unexpected filter captured: %+v", capturedFilter)
	}

	for _, r := range recRepo.recipients {
		if r.ExternalUserID != "u-admin" || r.RecipientEmail != "admin1@example.com" {
			t.Errorf("unexpected recipient: %+v", r)
		}
	}
}

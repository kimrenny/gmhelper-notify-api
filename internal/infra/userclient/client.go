package userclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/infra/auth"
)

var (
	ErrNotFound             = errors.New("user not found")
	ErrInvalidInput         = errors.New("invalid user id")
	ErrServer               = errors.New("gmhelper-api server error")
	ErrUnexpected           = errors.New("unexpected response from gmhelper-api")
	ErrAPIUnsuccessful      = errors.New("gmhelper-api returned unsuccessful response")
	ErrMissingTokenProvider = errors.New("tokenProvider cannot be nil")
	ErrTokenAcquisition     = errors.New("failed to obtain service token")
)

// User represents the authoritative user profile returned by gmhelper-api.
type User struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	Email            string    `json:"email"`
	Role             string    `json:"role"`
	Language         string    `json:"language"`
	IsActive         bool      `json:"isActive"`
	IsBlocked        bool      `json:"isBlocked"`
	RegistrationDate time.Time `json:"registrationDate"`
}

type apiResponse[T any] struct {
	Success bool    `json:"success"`
	Message *string `json:"message"`
	Data    *T      `json:"data"`
}

// Client defines the interface for resolving user information from gmhelper-api.
type Client interface {
	GetUserByID(ctx context.Context, id string) (*User, error)
	SearchUsers(ctx context.Context, query string, limit int) ([]User, error)
}

// HTTPClient implements Client using HTTP requests to gmhelper-api.
type HTTPClient struct {
	baseURL       string
	httpClient    *http.Client
	tokenProvider auth.ServiceTokenProvider
}

const defaultTimeout = 10 * time.Second

// NewClient constructs a new HTTPClient with the given baseURL, optional http.Client, and required auth.ServiceTokenProvider.
// If httpClient is nil, a default http.Client with a 10-second timeout is used.
func NewClient(baseURL string, httpClient *http.Client, tokenProvider auth.ServiceTokenProvider) (*HTTPClient, error) {
	trimmedURL := strings.TrimSpace(baseURL)
	if trimmedURL == "" {
		return nil, errors.New("baseURL cannot be empty")
	}

	parsed, err := url.ParseRequestURI(trimmedURL)
	if err != nil {
		return nil, fmt.Errorf("invalid baseURL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid baseURL scheme: %s (must be http or https)", parsed.Scheme)
	}

	if parsed.Host == "" {
		return nil, errors.New("baseURL must have a host")
	}

	if tokenProvider == nil {
		return nil, ErrMissingTokenProvider
	}

	cl := httpClient
	if cl == nil {
		cl = &http.Client{
			Timeout: defaultTimeout,
		}
	}

	return &HTTPClient{
		baseURL:       strings.TrimRight(trimmedURL, "/"),
		httpClient:    cl,
		tokenProvider: tokenProvider,
	}, nil
}

// GetUserByID queries gmhelper-api at GET /api/v1/internal/users/{id} and decodes the user profile.
func (c *HTTPClient) GetUserByID(ctx context.Context, id string) (*User, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil, fmt.Errorf("%w: user id cannot be empty", ErrInvalidInput)
	}

	token, err := c.tokenProvider.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenAcquisition, err)
	}
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return nil, fmt.Errorf("%w: returned empty token", ErrTokenAcquisition)
	}

	endpoint := fmt.Sprintf("%s/api/v1/internal/users/%s", c.baseURL, url.PathEscape(trimmedID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+trimmedToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to gmhelper-api failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		var errEnvelope apiResponse[any]
		if len(bodyBytes) > 0 && json.Unmarshal(bodyBytes, &errEnvelope) == nil && errEnvelope.Message != nil && *errEnvelope.Message != "" {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, *errEnvelope.Message)
		}
		return nil, ErrNotFound
	}

	if resp.StatusCode >= 500 {
		var errEnvelope apiResponse[any]
		if len(bodyBytes) > 0 && json.Unmarshal(bodyBytes, &errEnvelope) == nil && errEnvelope.Message != nil && *errEnvelope.Message != "" {
			return nil, fmt.Errorf("%w: status %d: %s", ErrServer, resp.StatusCode, *errEnvelope.Message)
		}
		return nil, fmt.Errorf("%w: status %d", ErrServer, resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errEnvelope apiResponse[any]
		if len(bodyBytes) > 0 && json.Unmarshal(bodyBytes, &errEnvelope) == nil && errEnvelope.Message != nil && *errEnvelope.Message != "" {
			return nil, fmt.Errorf("%w: status %d: %s", ErrUnexpected, resp.StatusCode, *errEnvelope.Message)
		}
		return nil, fmt.Errorf("%w: status %d", ErrUnexpected, resp.StatusCode)
	}

	var envelope apiResponse[User]
	if err := json.Unmarshal(bodyBytes, &envelope); err != nil {
		return nil, fmt.Errorf("%w: failed to decode response JSON: %v", ErrUnexpected, err)
	}

	if !envelope.Success {
		msg := "operation unsuccessful"
		if envelope.Message != nil && *envelope.Message != "" {
			msg = *envelope.Message
		}
		return nil, fmt.Errorf("%w: %s", ErrAPIUnsuccessful, msg)
	}

	if envelope.Data == nil {
		return nil, fmt.Errorf("%w: response data is nil", ErrUnexpected)
	}

	return envelope.Data, nil
}

// SearchUsers queries gmhelper-api at GET /api/v1/internal/users/search?query={query}&limit={limit} and decodes matching users.
func (c *HTTPClient) SearchUsers(ctx context.Context, query string, limit int) ([]User, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, fmt.Errorf("%w: search query cannot be empty", ErrInvalidInput)
	}

	token, err := c.tokenProvider.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenAcquisition, err)
	}
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return nil, fmt.Errorf("%w: returned empty token", ErrTokenAcquisition)
	}

	params := url.Values{}
	params.Set("query", trimmedQuery)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	endpoint := fmt.Sprintf("%s/api/v1/internal/users/search?%s", c.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+trimmedToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to gmhelper-api failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 500 {
		var errEnvelope apiResponse[any]
		if len(bodyBytes) > 0 && json.Unmarshal(bodyBytes, &errEnvelope) == nil && errEnvelope.Message != nil && *errEnvelope.Message != "" {
			return nil, fmt.Errorf("%w: status %d: %s", ErrServer, resp.StatusCode, *errEnvelope.Message)
		}
		return nil, fmt.Errorf("%w: status %d", ErrServer, resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errEnvelope apiResponse[any]
		if len(bodyBytes) > 0 && json.Unmarshal(bodyBytes, &errEnvelope) == nil && errEnvelope.Message != nil && *errEnvelope.Message != "" {
			return nil, fmt.Errorf("%w: status %d: %s", ErrUnexpected, resp.StatusCode, *errEnvelope.Message)
		}
		return nil, fmt.Errorf("%w: status %d", ErrUnexpected, resp.StatusCode)
	}

	var envelope apiResponse[[]User]
	if err := json.Unmarshal(bodyBytes, &envelope); err != nil {
		return nil, fmt.Errorf("%w: failed to decode response JSON: %v", ErrUnexpected, err)
	}

	if !envelope.Success {
		msg := "operation unsuccessful"
		if envelope.Message != nil && *envelope.Message != "" {
			msg = *envelope.Message
		}
		return nil, fmt.Errorf("%w: %s", ErrAPIUnsuccessful, msg)
	}

	if envelope.Data == nil {
		return nil, fmt.Errorf("%w: response data is nil", ErrUnexpected)
	}

	return *envelope.Data, nil
}

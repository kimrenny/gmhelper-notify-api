package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/user"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

// UserResponse represents the user data returned by the search endpoint.
type UserResponse struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	Email            string    `json:"email"`
	Role             string    `json:"role"`
	Language         string    `json:"language"`
	IsActive         bool      `json:"isActive"`
	IsBlocked        bool      `json:"isBlocked"`
	RegistrationDate time.Time `json:"registrationDate"`
}

// UserHandler handles user resolution and discovery endpoints.
type UserHandler struct {
	userService *user.Service
	logger      logger.Logger
}

// NewUserHandler constructs a new UserHandler.
func NewUserHandler(userService *user.Service, logger logger.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		logger:      logger,
	}
}

// Search handles GET /api/v1/users/search?q={query}&limit={limit}.
func (h *UserHandler) Search(w http.ResponseWriter, r *http.Request) {
	if h.userService == nil {
		h.logger.Error("user resolution service is not configured")
		response.Error(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "user service is not available")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "query parameter 'q' is required")
		return
	}

	limit := user.DefaultSearchLimit
	if limitStr := strings.TrimSpace(r.URL.Query().Get("limit")); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "limit must be a positive integer")
			return
		}
		limit = parsedLimit
	}

	users, err := h.userService.SearchUsers(r.Context(), query, limit)
	if err != nil {
		if errors.Is(err, user.ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		h.logger.Error("failed to search users from gmhelper-api", logger.Error(err), logger.String("query", query))
		response.Error(w, http.StatusBadGateway, "UPSTREAM_ERROR", "failed to search users from gmhelper-api")
		return
	}

	result := make([]UserResponse, 0, len(users))
	for _, u := range users {
		result = append(result, UserResponse{
			ID:               u.ID,
			Username:         u.Username,
			Email:            u.Email,
			Role:             u.Role,
			Language:         u.Language,
			IsActive:         u.IsActive,
			IsBlocked:        u.IsBlocked,
			RegistrationDate: u.RegistrationDate,
		})
	}

	response.JSON(w, http.StatusOK, result)
}

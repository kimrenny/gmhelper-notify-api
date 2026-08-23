package domain

// Principal represents the authenticated caller identity verified by service-to-service auth.
type Principal struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

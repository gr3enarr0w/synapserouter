package subscription

import (
	"strings"
	"time"
)

// ModelDefinition represents the payload for an individual subscription model.
type ModelDefinition struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	DisplayName    string     `json:"display_name,omitempty"`
	Description    string     `json:"description,omitempty"`
	Status         string     `json:"status,omitempty"`
	Roles          []string   `json:"roles,omitempty"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	MaxConcurrency int        `json:"max_concurrency,omitempty"`
}

// SupportsRole determines whether the model advertises support for the provided role.
func (m ModelDefinition) SupportsRole(role string) bool {
	if role == "" {
		return true
	}
	for _, candidate := range m.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

// IsAvailable checks whether the model is currently marked available.
func (m ModelDefinition) IsAvailable() bool {
	return m.Status == "" || strings.EqualFold(m.Status, "available")
}

// ModelListResponse reflects the API response for GET /api/subscription/v1/models.
type ModelListResponse struct {
	Models    []ModelDefinition `json:"models"`
	UpdatedAt *time.Time        `json:"updated_at,omitempty"`
}

// ModelSelection represents the router-friendly model metadata returned from the subscription package.
type ModelSelection struct {
	Model ModelDefinition
	Role  string
}
package atlas

import "fmt"

// Config holds Atlas Admin API client settings.
type Config struct {
	PublicKey   string
	PrivateKey  string
	BaseURL     string
	RateLimitMS int
}

// Project identifies a single Atlas project (group).
type Project struct {
	ID   string
	Name string
}

// Cluster captures Atlas cluster metadata used in detections.
type Cluster struct {
	ID               string
	Name             string
	MongoDBVersion   string
	InstanceSizeName string
}

// SuggestedIndex is a single Atlas Performance Advisor recommendation.
type SuggestedIndex struct {
	Namespace   string
	IndexFields []string
}

// Alert captures Atlas alert state used for reporting.
type Alert struct {
	ID            string
	EventTypeName string
	Status        string
}

// APIError is a structured non-2xx Atlas API response.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return "atlas api error"
	}
	if e.Code != "" {
		return fmt.Sprintf("atlas api %d (%s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("atlas api %d: %s", e.StatusCode, e.Message)
}

// IsStatus reports whether err is an APIError with the given status code.
func IsStatus(err error, status int) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == status
}

// DatabaseUser represents an Atlas database user from the Admin API.
type DatabaseUser struct {
	Username        string              `json:"username"`
	DatabaseName    string              `json:"databaseName"`
	Roles           []DatabaseUserRole  `json:"roles"`
	Scopes          []DatabaseUserScope `json:"scopes"`
	GroupID         string              `json:"groupId"`
	DeleteAfterDate string              `json:"deleteAfterDate,omitempty"`
}

// DatabaseUserRole is a single role assignment for an Atlas database user.
type DatabaseUserRole struct {
	RoleName     string `json:"roleName"`
	DatabaseName string `json:"databaseName"`
}

// DatabaseUserScope restricts an Atlas database user to specific clusters or data lakes.
type DatabaseUserScope struct {
	Name string `json:"name"`
	Type string `json:"type"` // "CLUSTER" or "DATA_LAKE"
}

package client

import (
	"encoding/json"
	"fmt"
	"time"
)

// Project represents a Fogpipe project.
type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Egress     string    `json:"egress"`
	Plan       string    `json:"plan"`
	IsPlatform bool      `json:"is_platform,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateProjectRequest is the request body for creating a project.
type CreateProjectRequest struct {
	Name   string `json:"name"`
	Egress string `json:"egress,omitempty"`
	Plan   string `json:"plan,omitempty"`
}

// AuditEntry is one record from the audit log.
type AuditEntry struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"ts"`
	ActorType    string         `json:"actor_type"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Details      map[string]any `json:"details,omitempty"`
}

// UpdateProjectRequest is the request body for updating a project.
type UpdateProjectRequest struct {
	Egress string `json:"egress,omitempty"`
	Plan   string `json:"plan,omitempty"`
}

// TrustBinding is a per-project OIDC federation trust binding: a repo (matched by
// SubjectPattern) on Issuer, carrying Audience, may assume ServiceAccountID.
type TrustBinding struct {
	ID               string    `json:"id"`
	Issuer           string    `json:"issuer"`
	Audience         string    `json:"audience"`
	SubjectPattern   string    `json:"subject_pattern"`
	ServiceAccountID string    `json:"service_account_id"`
	TokenTTLSeconds  int       `json:"token_ttl_seconds"`
	CreatedAt        time.Time `json:"created_at"`
}

// CreateTrustBindingRequest is the request body for creating a trust binding.
type CreateTrustBindingRequest struct {
	Issuer          string `json:"issuer"`
	Audience        string `json:"audience"`
	SubjectPattern  string `json:"subject_pattern"`
	ServiceAccount  string `json:"service_account"`
	TokenTTLSeconds int    `json:"token_ttl_seconds,omitempty"`
}

// App represents a deployed application.
type App struct {
	ID                  string    `json:"id"`
	ProjectID           string    `json:"project_id"`
	Name                string    `json:"name"`
	Image               string    `json:"image"`
	Status              string    `json:"status"`
	URL                 string    `json:"url"`
	Domains             []string  `json:"domains"`
	Replicas            int       `json:"replicas"`
	MinScale            int32     `json:"min_scale"`
	MaxScale            int32     `json:"max_scale"`
	CPULimit            string    `json:"cpu_limit"`
	MemoryLimit         string    `json:"memory_limit"`
	Ingress             string    `json:"ingress"`
	Tier                string    `json:"tier"`
	Storage             string    `json:"storage"`
	StoragePath         string    `json:"storage_path"`
	ServiceAccountID    string    `json:"service_account_id,omitempty"`
	HealthCheckPath     string    `json:"health_check_path"`
	HealthCheckTimeout  int       `json:"health_check_timeout"`
	HealthCheckInterval int       `json:"health_check_interval"`
	HealthCheckRetries  int       `json:"health_check_retries"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// CreateAppRequest is the request body for creating an app.
type CreateAppRequest struct {
	Name                string            `json:"name"`
	Image               string            `json:"image"`
	Port                int               `json:"port,omitempty"`
	Replicas            int               `json:"replicas,omitempty"`
	Ingress             string            `json:"ingress,omitempty"`
	Tier                string            `json:"tier,omitempty"`         // "dedicated" (default) or "serverless"
	Storage             string            `json:"storage,omitempty"`      // persistent volume size (e.g. "50Gi")
	StoragePath         string            `json:"storage_path,omitempty"` // mount path (defaults to /data)
	EnvVars             map[string]string `json:"env_vars,omitempty"`
	ServiceAccount      string            `json:"service_account,omitempty"` // SA email or ID
	HealthCheckPath     string            `json:"health_check_path,omitempty"`
	HealthCheckTimeout  int               `json:"health_check_timeout,omitempty"`
	HealthCheckInterval int               `json:"health_check_interval,omitempty"`
	HealthCheckRetries  int               `json:"health_check_retries,omitempty"`
}

// DeployRequest is the request body for deploying a new app revision.
type DeployRequest struct {
	Image     string `json:"image"`
	NoTraffic bool   `json:"no_traffic,omitempty"`
}

// TrafficTarget represents a traffic routing target.
type TrafficTarget struct {
	Revision string `json:"revision"`
	Percent  int64  `json:"percent"`
	URL      string `json:"url,omitempty"`
}

// SetTrafficRequest is the request body for setting traffic split.
type SetTrafficRequest struct {
	Targets []TrafficTarget `json:"targets"`
}

// TrafficResponse is the response for traffic operations.
type TrafficResponse struct {
	Targets []TrafficTarget `json:"targets"`
}

// ScaleRequest is the request body for scaling an app.
type ScaleRequest struct {
	MinScale    *int32 `json:"min_scale,omitempty"`
	MaxScale    *int32 `json:"max_scale,omitempty"`
	CPULimit    string `json:"cpu_limit,omitempty"`
	MemoryLimit string `json:"memory_limit,omitempty"`
}

// SwitchTierRequest is the request body for switching an app's hosting tier.
type SwitchTierRequest struct {
	Tier string `json:"tier"`
}

// UpdateStorageRequest is the request body for growing an app's persistent storage.
type UpdateStorageRequest struct {
	Storage string `json:"storage"`
}

// RollbackRequest is the request body for rolling back an app.
type RollbackRequest struct {
	Revision string `json:"revision,omitempty"`
}

// Database represents a managed database instance.
type Database struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	Name             string    `json:"name"`
	Engine           string    `json:"engine"`
	Version          string    `json:"version"`
	Plan             string    `json:"plan"`
	Status           string    `json:"status"`
	ConnectionString string    `json:"connection_string"`
	Pooler           bool      `json:"pooler"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// CreateDatabaseRequest is the request body for creating a database.
type CreateDatabaseRequest struct {
	Name    string `json:"name"`
	Engine  string `json:"engine"`
	Version string `json:"version,omitempty"`
	CPU     string `json:"cpu,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Storage string `json:"storage,omitempty"`
	Pooler  bool   `json:"pooler,omitempty"`
}

// UpdateDatabaseRequest is the request body for reconciling a database's spec.
// Empty strings and nil pointers mean "leave unchanged".
type UpdateDatabaseRequest struct {
	CPU       string `json:"cpu,omitempty"`
	Memory    string `json:"memory,omitempty"`
	Storage   string `json:"storage,omitempty"`
	Version   string `json:"version,omitempty"`
	Instances *int64 `json:"instances,omitempty"`
	Pooler    *bool  `json:"pooler,omitempty"`
}

// Domain represents a custom domain attached to an application.
type Domain struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	Domain    string    `json:"domain"`
	Status    string    `json:"status"`
	TLSStatus string    `json:"tls_status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DomainRequest is the request body for adding or removing a domain.
type DomainRequest struct {
	Domain string `json:"domain"`
}

// AppConfig represents an environment variable or secret for an application.
type AppConfig struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	IsSecret  bool      `json:"is_secret"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SetConfigRequest is the request body for setting a config value.
type SetConfigRequest struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"is_secret"`
}

// Revision represents a Knative revision for an application.
type Revision struct {
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Image     string `json:"image"`
	CreatedAt string `json:"created_at"`
}

// User represents a registered platform user.
type User struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	OrganizationID string    `json:"organization_id"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// Organization represents a platform organization.
type Organization struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// SetupWebhookRequest is the request body for setting up a webhook.
type SetupWebhookRequest struct {
	Repo         string `json:"repo"`
	Branch       string `json:"branch"`
	ImagePattern string `json:"image_pattern"`
}

// AppWebhook represents a webhook configuration (returned from setup).
type AppWebhook struct {
	ID            string  `json:"id"`
	AppID         string  `json:"app_id"`
	Provider      string  `json:"provider"`
	Repo          string  `json:"repo"`
	Branch        string  `json:"branch"`
	ImagePattern  string  `json:"image_pattern"`
	Enabled       bool    `json:"enabled"`
	WebhookURL    string  `json:"webhook_url"`
	WebhookSecret string  `json:"webhook_secret,omitempty"`
	LastDeployAt  *string `json:"last_deploy_at,omitempty"`
	LastDeploySHA string  `json:"last_deploy_sha,omitempty"`
}

// RegisterRequest is the request body for user registration.
type RegisterRequest struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	OrgName string `json:"org_name,omitempty"`
}

// RegisterResponse is the response from user registration.
type RegisterResponse struct {
	User         *User         `json:"user"`
	Organization *Organization `json:"organization"`
	APIKey       string        `json:"api_key"`
}

// ProvisionUserRequest is the request body for admin-provisioning a user
// into an existing organization (POST /api/v1/orgs/{orgID}/users).
type ProvisionUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role,omitempty"`
}

// MeResponse is the response from the /auth/me endpoint.
type MeResponse struct {
	User         *User         `json:"user"`
	Organization *Organization `json:"organization"`
}

// ServiceAccount represents a service account.
type ServiceAccount struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateServiceAccountRequest is the request body for creating a service account.
type CreateServiceAccountRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
}

// ServiceAccountKey represents a service account key.
type ServiceAccountKey struct {
	ID               string  `json:"id"`
	ServiceAccountID string  `json:"service_account_id"`
	APIKey           string  `json:"api_key,omitempty"`
	Prefix           string  `json:"prefix"`
	CreatedAt        string  `json:"created_at"`
	ExpiresAt        *string `json:"expires_at,omitempty"`
}

// IAMBinding represents an IAM role binding.
type IAMBinding struct {
	ID           string `json:"id"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Role         string `json:"role"`
	MemberType   string `json:"member_type"`
	Member       string `json:"member"`
	CreatedAt    string `json:"created_at"`
}

// OrgMember represents a member of an organization.
type OrgMember struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id"`
	Role           string `json:"role"`
	InvitedBy      string `json:"invited_by,omitempty"`
	InvitedEmail   string `json:"invited_email,omitempty"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	UserEmail      string `json:"user_email,omitempty"`
	UserName       string `json:"user_name,omitempty"`
}

// InviteOrgMemberRequest is the request body for inviting a member to an organization.
type InviteOrgMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// UpdateOrgMemberRoleRequest is the request body for updating a member's role.
type UpdateOrgMemberRoleRequest struct {
	Role string `json:"role"`
}

// DatabaseBackup represents a backup of a managed database.
type DatabaseBackup struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at,omitempty"`
	StoppedAt string `json:"stopped_at,omitempty"`
}

// BackupConfig represents the backup configuration for a database.
type BackupConfig struct {
	Enabled   bool   `json:"enabled"`
	Schedule  string `json:"schedule"`
	Retention string `json:"retention"`
}

// RestoreRequest is the request body for restoring a database from backup.
type RestoreRequest struct {
	PointInTime string `json:"point_in_time,omitempty"`
	TargetName  string `json:"target_name"`
}

// Deployment represents a single deployment event for an application.
type Deployment struct {
	ID         string  `json:"id"`
	AppID      string  `json:"app_id"`
	Image      string  `json:"image"`
	Status     string  `json:"status"`
	Trigger    string  `json:"trigger"`
	CommitSHA  string  `json:"commit_sha,omitempty"`
	Message    string  `json:"message,omitempty"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at,omitempty"`
	DurationMs *int    `json:"duration_ms,omitempty"`
	CreatedBy  string  `json:"created_by,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

// SetIAMBindingRequest is the request body for setting an IAM binding.
type SetIAMBindingRequest struct {
	Role       string `json:"role"`
	MemberType string `json:"member_type"`
	Member     string `json:"member"`
}

// APIError represents an error response from the API.
// It supports both the new nested format {"error":{"code":"...","message":"..."}}
// and the legacy flat format {"error":"message"}.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

// UnmarshalJSON implements custom JSON unmarshaling to handle both the new
// nested error format and the legacy flat format.
func (e *APIError) UnmarshalJSON(data []byte) error {
	// Try nested format: {"error": {"code": "...", "message": "..."}}
	var nested struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &nested); err == nil && nested.Error.Message != "" {
		e.Code = nested.Error.Code
		e.Message = nested.Error.Message
		return nil
	}

	// Try flat format: {"error": "message"}
	var flat struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &flat); err == nil && flat.Error != "" {
		e.Message = flat.Error
		return nil
	}

	// Try message format: {"message": "..."}
	var msg struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &msg); err == nil && msg.Message != "" {
		e.Message = msg.Message
		return nil
	}

	return fmt.Errorf("unknown error format")
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

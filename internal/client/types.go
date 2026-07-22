package client

import (
	"encoding/json"
	"fmt"
	"time"
)

// Project represents a Fogpipe project.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Namespace   string    `json:"namespace"`
	Egress      string    `json:"egress"`
	MaxCPU      string    `json:"max_cpu"`
	MaxMemory   string    `json:"max_memory"`
	MaxPods     int       `json:"max_pods"`
	MaxStorage  string    `json:"max_storage"`
	IsPlatform  bool      `json:"is_platform,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateProjectRequest is the request body for creating a project.
type CreateProjectRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Egress      string `json:"egress,omitempty"`
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
	DisplayName string  `json:"display_name,omitempty"`
	Egress      string  `json:"egress,omitempty"`
	MaxCPU      *string `json:"max_cpu,omitempty"`
	MaxMemory   *string `json:"max_memory,omitempty"`
	MaxPods     *int    `json:"max_pods,omitempty"`
	MaxStorage  *string `json:"max_storage,omitempty"`
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
	DisplayName         string    `json:"display_name"`
	URLSlug             string    `json:"url_slug"` // optional vanity host override (ADR-040); empty = derived host
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
	Mode                string    `json:"mode"`
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

// VolumeMount mounts a ConfigMap/Secret as read-only files, or an emptyDir as
// writable scratch, at a container path.
type VolumeMount struct {
	Source    string `json:"source"`             // "configmap", "secret", or "emptydir"
	Name      string `json:"name"`               // ConfigMap/Secret name (ignored for emptydir)
	MountPath string `json:"mount_path"`         // container path to mount at
	SubPath   string `json:"sub_path,omitempty"` // mount a single key instead of the whole dir
}

// SecurityContext hardens an app's pod/container (nil = image default).
type SecurityContext struct {
	RunAsUser              *int64 `json:"run_as_user,omitempty"`
	RunAsGroup             *int64 `json:"run_as_group,omitempty"`
	FSGroup                *int64 `json:"fs_group,omitempty"`
	RunAsNonRoot           bool   `json:"run_as_non_root,omitempty"`
	ReadOnlyRootFilesystem bool   `json:"read_only_root_filesystem,omitempty"`
}

// CreateAppRequest is the request body for creating an app.
type CreateAppRequest struct {
	Name                string            `json:"name"`
	DisplayName         string            `json:"display_name,omitempty"` // mutable cosmetic label; defaults to Name
	URLSlug             string            `json:"url_slug,omitempty"`     // optional vanity host override (ADR-040)
	Image               string            `json:"image"`
	Command             []string          `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	ReleaseCommand      []string          `json:"release_command,omitempty"` // run once per deploy, before the new version goes live
	VolumeMounts        []VolumeMount     `json:"volume_mounts,omitempty"`
	SecurityContext     *SecurityContext  `json:"security_context,omitempty"`
	Port                int               `json:"port,omitempty"`
	Replicas            int               `json:"replicas,omitempty"`
	Ingress             string            `json:"ingress,omitempty"`
	Mode                string            `json:"mode,omitempty"`         // "always-on" (default) or "serverless"
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
	Replicas    *int32 `json:"replicas,omitempty"`
	CPULimit    string `json:"cpu_limit,omitempty"`
	MemoryLimit string `json:"memory_limit,omitempty"`
}

// SwitchModeRequest is the request body for switching an app's hosting mode.
type SwitchModeRequest struct {
	Mode string `json:"mode"`
}

// UpdateStorageRequest is the request body for growing an app's persistent storage.
type UpdateStorageRequest struct {
	Storage string `json:"storage"`
}

// UpdateAppRequest is the request body for PATCH /api/v1/apps/{appID}. Both fields
// are optional: display_name changes the app's cosmetic label (the frozen name is
// not renamable in place); url_slug sets or clears the optional vanity host override
// (ADR-040) — a non-nil pointer to "" clears it back to the derived host.
type UpdateAppRequest struct {
	DisplayName string  `json:"display_name,omitempty"`
	URLSlug     *string `json:"url_slug,omitempty"`
}

// UpdateCommandRequest is the request body for changing an app's container
// entrypoint override and arguments. Each field is optional: a nil pointer leaves
// the value untouched, a non-nil pointer (including an empty array) replaces it —
// an empty array clears the override back to the image defaults.
type UpdateCommandRequest struct {
	Command        *[]string `json:"command,omitempty"`
	Args           *[]string `json:"args,omitempty"`
	ReleaseCommand *[]string `json:"release_command,omitempty"`
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
	DisplayName      string    `json:"display_name"`
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
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Engine      string `json:"engine"`
	Version     string `json:"version,omitempty"`
	CPU         string `json:"cpu,omitempty"`
	Memory      string `json:"memory,omitempty"`
	Storage     string `json:"storage,omitempty"`
	Pooler      bool   `json:"pooler,omitempty"`
}

// UpdateDatabaseRequest is the request body for reconciling a database's spec.
// Empty strings and nil pointers mean "leave unchanged".
type UpdateDatabaseRequest struct {
	DisplayName string `json:"display_name,omitempty"`
	CPU         string `json:"cpu,omitempty"`
	Memory      string `json:"memory,omitempty"`
	Storage     string `json:"storage,omitempty"`
	Version     string `json:"version,omitempty"`
	Instances   *int64 `json:"instances,omitempty"`
	Pooler      *bool  `json:"pooler,omitempty"`
}

// Bucket is a managed S3 object-storage bucket on the Garage store (ADR-039).
// SecretAccessKey is only populated on creation.
type Bucket struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	Name            string    `json:"name"`
	GarageBucketID  string    `json:"garage_bucket_id,omitempty"`
	AccessKeyID     string    `json:"access_key_id,omitempty"`
	SecretAccessKey string    `json:"secret_access_key,omitempty"`
	GlobalAlias     string    `json:"global_alias,omitempty"`
	Region          string    `json:"region,omitempty"`
	Endpoint        string    `json:"endpoint,omitempty"`
	QuotaMaxSize    int64     `json:"quota_max_size,omitempty"`
	QuotaMaxObjects int64     `json:"quota_max_objects,omitempty"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Static-website serving (#342). When enabled the bucket is served
	// anonymously over HTTP (public read) at WebsiteURL.
	WebsiteEnabled       bool   `json:"website_enabled"`
	WebsiteIndexDocument string `json:"website_index_document,omitempty"`
	WebsiteErrorDocument string `json:"website_error_document,omitempty"`
	URLSlug              string `json:"url_slug"`
	WebsiteURL           string `json:"website_url,omitempty"`
}

// CreateBucketRequest is the request body for creating a bucket.
type CreateBucketRequest struct {
	Name            string `json:"name"`
	QuotaMaxSize    int64  `json:"quota_max_size,omitempty"`
	QuotaMaxObjects int64  `json:"quota_max_objects,omitempty"`
}

// SetBucketQuotaRequest is the request body for updating a bucket's quotas.
type SetBucketQuotaRequest struct {
	QuotaMaxSize    int64 `json:"quota_max_size"`
	QuotaMaxObjects int64 `json:"quota_max_objects"`
}

// SetBucketWebsiteRequest is the request body for toggling static-website
// serving on a bucket (#342). Enabling makes the bucket world-readable over
// HTTP; the index/error documents are optional (index defaults to index.html).
type SetBucketWebsiteRequest struct {
	Enabled       bool   `json:"enabled"`
	IndexDocument string `json:"index_document,omitempty"`
	ErrorDocument string `json:"error_document,omitempty"`
}

// SetBucketURLSlugRequest is the request body for setting (or clearing, with
// "") a bucket website's vanity host label.
type SetBucketURLSlugRequest struct {
	URLSlug string `json:"url_slug"`
}

// BucketKey is a scoped S3 access key for a bucket. SecretAccessKey is only
// populated when the key is created.
type BucketKey struct {
	ID              string    `json:"id"`
	BucketID        string    `json:"bucket_id"`
	AccessKeyID     string    `json:"access_key_id"`
	Name            string    `json:"name,omitempty"`
	CanRead         bool      `json:"can_read"`
	CanWrite        bool      `json:"can_write"`
	CanOwner        bool      `json:"can_owner"`
	SecretAccessKey string    `json:"secret_access_key,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateBucketKeyRequest is the request body for minting a scoped access key.
type CreateBucketKeyRequest struct {
	Name  string `json:"name,omitempty"`
	Read  bool   `json:"read"`
	Write bool   `json:"write"`
	Owner bool   `json:"owner"`
}

// UpdateBucketKeyPermissionsRequest is the request body for changing a key's grants.
type UpdateBucketKeyPermissionsRequest struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
	Owner bool `json:"owner"`
}

// AppBucketBinding is an explicit app ⇄ bucket binding (#264). Binding injects the
// bucket's S3_*/AWS_* credentials into the app's pod via a k8s Secret + envFrom.
// The secret access key is never returned.
type AppBucketBinding struct {
	AppID       string    `json:"app_id"`
	BucketID    string    `json:"bucket_id"`
	BucketName  string    `json:"bucket_name,omitempty"`
	Endpoint    string    `json:"endpoint,omitempty"`
	Region      string    `json:"region,omitempty"`
	ReadOnly    bool      `json:"read_only"`
	AccessKeyID string    `json:"access_key_id,omitempty"`
	SecretName  string    `json:"secret_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// BindBucketRequest is the request body for binding a bucket to an app.
type BindBucketRequest struct {
	BucketID string `json:"bucket_id"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// BucketCredentials are the S3 connection details for a bucket. SecretAccessKey
// is only present when a fresh key was minted.
type BucketCredentials struct {
	Bucket          string `json:"bucket"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	Note            string `json:"note,omitempty"`
}

// ObjectInfo is a single stored object in the in-browser object browser (#268).
type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// ObjectListing is one page of a bucket's objects under a prefix; Prefixes are
// the "folder" common-prefixes when a delimiter is used (#268).
type ObjectListing struct {
	Prefixes []string     `json:"prefixes"`
	Objects  []ObjectInfo `json:"objects"`
}

// PresignObjectRequest is the request body for minting a presigned object URL.
type PresignObjectRequest struct {
	Key     string `json:"key"`
	Method  string `json:"method"`            // GET (download) or PUT (upload)
	Expires int    `json:"expires,omitempty"` // seconds; clamped server-side
}

// PresignResponse is a presigned S3 URL the browser uses to GET/PUT an object
// directly against the object store — bytes never transit the API (#268).
type PresignResponse struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Expires int               `json:"expires"`
}

// Domain represents a custom domain attached to an application.
type Domain struct {
	ID                string     `json:"id"`
	AppID             string     `json:"app_id,omitempty"`
	BucketID          string     `json:"bucket_id,omitempty"`
	Domain            string     `json:"domain"`
	Status            string     `json:"status"`
	TLSStatus         string     `json:"tls_status"`
	VerificationToken string     `json:"verification_token,omitempty"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// DomainRequest is the request body for adding or removing a domain.
type DomainRequest struct {
	Domain string `json:"domain"`
}

// DomainVerification is the ownership/pointing/cert breakdown for a custom
// domain plus the exact DNS records the tenant still needs to configure.
type DomainVerification struct {
	Domain         *Domain `json:"domain"`
	TXTVerified    bool    `json:"txt_verified"`
	DNSPointing    bool    `json:"dns_pointing"`
	CertReady      bool    `json:"cert_ready"`
	CertReason     string  `json:"cert_reason,omitempty"`
	CertExpiry     string  `json:"cert_expiry,omitempty"`
	TXTRecordName  string  `json:"txt_record_name"`
	TXTRecordValue string  `json:"txt_record_value"`
	PointingType   string  `json:"pointing_type"`
	PointingName   string  `json:"pointing_name"`
	PointingValue  string  `json:"pointing_value"`
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
	ShortID     string    `json:"short_id"`
	DisplayName string    `json:"display_name"`
	FKEEnabled  bool      `json:"fke_enabled"` // operator-granted entitlement gating FKE/kubectl access
	CreatedAt   time.Time `json:"created_at"`
}

// UpdateOrgRequest is the request body for updating an organization. FKEEnabled is
// a pointer so an omitted field is distinguishable from an explicit false.
type UpdateOrgRequest struct {
	FKEEnabled *bool `json:"fke_enabled,omitempty"`
}

// OrgSecret is a Fogpipe Secrets Manager bundle (ADR-028): an org-scoped named
// set of key/value entries. Data is populated only on an explicit reveal.
type OrgSecret struct {
	ID        string            `json:"id"`
	OrgID     string            `json:"org_id"`
	Name      string            `json:"name"`
	Keys      []string          `json:"keys"`
	Data      map[string]string `json:"data,omitempty"`
	Targets   []string          `json:"targets"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
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
	Method    string `json:"method,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	StoppedAt string `json:"stopped_at,omitempty"`
}

// BackupConfig represents the backup configuration for a database.
type BackupConfig struct {
	Enabled                  bool   `json:"enabled"`
	Schedule                 string `json:"schedule"`
	Retention                string `json:"retention"`
	FirstRecoverabilityPoint string `json:"first_recoverability_point,omitempty"`
}

// BackupDestination is an opt-in, per-database external backup target (issue
// #130): the customer's own bucket (AWS or GCP) that a database backs up directly
// to, keyless via OIDC federation — no secrets, only non-secret identifiers (AWS
// role ARN, or GCP WIF provider + service account). Provider "aws" uses RoleARN;
// "gcp" uses WIFProvider + ServiceAccount.
type BackupDestination struct {
	Provider       string `json:"provider"` // "aws" | "gcp"
	Bucket         string `json:"bucket"`
	Region         string `json:"region,omitempty"`
	Prefix         string `json:"prefix,omitempty"`
	RoleARN        string `json:"role_arn,omitempty"`
	WIFProvider    string `json:"wif_provider,omitempty"`
	ServiceAccount string `json:"service_account,omitempty"`
	Audience       string `json:"audience,omitempty"`
	Schedule       string `json:"schedule,omitempty"`
	Enabled        bool   `json:"enabled"`
	LastRunAt      string `json:"last_run_at,omitempty"`
	LastRunStatus  string `json:"last_run_status,omitempty"`
}

// BackupDestinationRun identifies an on-demand external backup that was started.
// The backup runs as an async k8s Job; Status reflects the launch, not completion.
type BackupDestinationRun struct {
	JobName string `json:"job_name"`
	Status  string `json:"status"`
	Subject string `json:"subject"`
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

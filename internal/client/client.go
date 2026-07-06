package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the Fogpipe API client.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// New creates a new API client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	return req, nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		if err := json.NewDecoder(resp.Body).Decode(apiErr); err != nil {
			return &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("HTTP %d", resp.StatusCode)}
		}
		return apiErr
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}

// Register creates a new user account. Does not require authentication.
func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/auth/register", req)
	if err != nil {
		return nil, err
	}
	// Remove auth header — registration is public.
	httpReq.Header.Del("Authorization")
	var resp RegisterResponse
	if err := c.do(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetMe retrieves the current user's info.
func (c *Client) GetMe(ctx context.Context) (*MeResponse, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/auth/me", nil)
	if err != nil {
		return nil, err
	}
	var resp MeResponse
	if err := c.do(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RegistryCredentials is a project-scoped registry credential for a tenant's CI.
type RegistryCredentials struct {
	Server     string `json:"server"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Repository string `json:"repository"`
}

// GetRegistryCredentials fetches the caller's project-scoped registry credential
// (a Zot user limited to tenants/<project>/**). The caller authenticates with a
// service-account API key — e.g. one minted by OIDC federation.
func (c *Client) GetRegistryCredentials(ctx context.Context) (*RegistryCredentials, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/registry/credentials", nil)
	if err != nil {
		return nil, err
	}
	var resp RegistryCredentials
	if err := c.do(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateProject creates a new project.
func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects", req)
	if err != nil {
		return nil, err
	}
	var project Project
	if err := c.do(httpReq, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// CreateProjectInOrg creates a new project under a specific organization.
func (c *Client) CreateProjectInOrg(ctx context.Context, orgID string, req CreateProjectRequest) (*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/orgs/"+orgID+"/projects", req)
	if err != nil {
		return nil, err
	}
	var project Project
	if err := c.do(httpReq, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// GetProject retrieves a project by ID.
func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+id, nil)
	if err != nil {
		return nil, err
	}
	var project Project
	if err := c.do(httpReq, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// ListProjects lists all projects the caller can access across every org (IAM-driven).
func (c *Client) ListProjects(ctx context.Context) ([]*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects", nil)
	if err != nil {
		return nil, err
	}
	var projects []*Project
	if err := c.do(httpReq, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// ListProjectsInOrg lists the projects the caller can access within a single org
// (org id or name). Scoped, unlike ListProjects.
func (c *Client) ListProjectsInOrg(ctx context.Context, orgID string) ([]*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/orgs/"+orgID+"/projects", nil)
	if err != nil {
		return nil, err
	}
	var projects []*Project
	if err := c.do(httpReq, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// UpdateProjectEgress sets a project's egress mode (restricted, https, all).
func (c *Client) UpdateProjectEgress(ctx context.Context, id, egress string) (*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/projects/"+id, UpdateProjectRequest{Egress: egress})
	if err != nil {
		return nil, err
	}
	var project Project
	if err := c.do(httpReq, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// UpdateProjectPlan sets a project's plan tier (starter, standard, premium).
func (c *Client) UpdateProjectPlan(ctx context.Context, id, plan string) (*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/projects/"+id, UpdateProjectRequest{Plan: plan})
	if err != nil {
		return nil, err
	}
	var project Project
	if err := c.do(httpReq, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// ListAudit returns audit log entries, optionally filtered by query params
// (resource_type, resource_id, actor, limit).
func (c *Client) ListAudit(ctx context.Context, query string) ([]*AuditEntry, error) {
	path := "/api/v1/audit"
	if query != "" {
		path += "?" + query
	}
	httpReq, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var entries []*AuditEntry
	if err := c.do(httpReq, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// DeleteProject deletes a project by ID.
func (c *Client) DeleteProject(ctx context.Context, id string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/projects/"+id, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// CreateApp creates a new app in a project.
func (c *Client) CreateApp(ctx context.Context, projectID string, req CreateAppRequest) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/apps", req)
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// GetApp retrieves an app by ID.
func (c *Client) GetApp(ctx context.Context, id string) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+id, nil)
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// ListApps lists all apps in a project.
func (c *Client) ListApps(ctx context.Context, projectID string) ([]*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/apps", nil)
	if err != nil {
		return nil, err
	}
	var apps []*App
	if err := c.do(httpReq, &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

// DeployApp deploys a new revision of an app.
func (c *Client) DeployApp(ctx context.Context, id string, req DeployRequest) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/apps/"+id+"/deploy", req)
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// ScaleApp updates the scaling configuration for an app.
func (c *Client) ScaleApp(ctx context.Context, id string, req ScaleRequest) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/apps/"+id+"/scale", req)
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// SwitchTier migrates an app between hosting tiers ("dedicated"/"serverless").
func (c *Client) SwitchTier(ctx context.Context, id, tier string) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/apps/"+id+"/tier", SwitchTierRequest{Tier: tier})
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// UpdateAppStorage grows an app's persistent volume (grow-only, dedicated tier).
func (c *Client) UpdateAppStorage(ctx context.Context, id, storage string) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/apps/"+id+"/storage", UpdateStorageRequest{Storage: storage})
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// RollbackApp rolls back an app to a previous revision.
func (c *Client) RollbackApp(ctx context.Context, id string, revision string) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/apps/"+id+"/rollback", RollbackRequest{Revision: revision})
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// DeleteApp deletes an app by ID.
func (c *Client) DeleteApp(ctx context.Context, id string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/apps/"+id, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// GetAppLogs retrieves logs for an app. If follow is true, the returned
// ReadCloser streams logs until closed.
func (c *Client) GetAppLogs(ctx context.Context, id string, follow bool) (io.ReadCloser, error) {
	path := "/api/v1/apps/" + id + "/logs"
	if follow {
		path += "?follow=true"
	}

	httpReq, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		apiErr := &APIError{StatusCode: resp.StatusCode}
		if err := json.NewDecoder(resp.Body).Decode(apiErr); err != nil {
			return nil, &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("HTTP %d", resp.StatusCode)}
		}
		return nil, apiErr
	}

	return resp.Body, nil
}

// GetTraffic retrieves the current traffic split for an app.
func (c *Client) GetTraffic(ctx context.Context, appID string) ([]TrafficTarget, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+appID+"/traffic", nil)
	if err != nil {
		return nil, err
	}
	var resp TrafficResponse
	if err := c.do(httpReq, &resp); err != nil {
		return nil, err
	}
	return resp.Targets, nil
}

// SetTraffic sets the traffic split for an app.
func (c *Client) SetTraffic(ctx context.Context, appID string, targets []TrafficTarget) ([]TrafficTarget, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/apps/"+appID+"/traffic", SetTrafficRequest{Targets: targets})
	if err != nil {
		return nil, err
	}
	var resp TrafficResponse
	if err := c.do(httpReq, &resp); err != nil {
		return nil, err
	}
	return resp.Targets, nil
}

// ListRevisions lists all revisions for an app.
func (c *Client) ListRevisions(ctx context.Context, appID string) ([]Revision, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+appID+"/revisions", nil)
	if err != nil {
		return nil, err
	}
	var revisions []Revision
	if err := c.do(httpReq, &revisions); err != nil {
		return nil, err
	}
	return revisions, nil
}

// CreateDatabase creates a new database in a project.
func (c *Client) CreateDatabase(ctx context.Context, projectID string, req CreateDatabaseRequest) (*Database, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/databases", req)
	if err != nil {
		return nil, err
	}
	var db Database
	if err := c.do(httpReq, &db); err != nil {
		return nil, err
	}
	return &db, nil
}

// GetDatabase retrieves a database by ID.
func (c *Client) GetDatabase(ctx context.Context, id string) (*Database, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/databases/"+id, nil)
	if err != nil {
		return nil, err
	}
	var db Database
	if err := c.do(httpReq, &db); err != nil {
		return nil, err
	}
	return &db, nil
}

// UpdateDatabase reconciles a database's spec (cpu/memory/storage/instances/version/pooler).
func (c *Client) UpdateDatabase(ctx context.Context, id string, req UpdateDatabaseRequest) (*Database, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/databases/"+id, req)
	if err != nil {
		return nil, err
	}
	var db Database
	if err := c.do(httpReq, &db); err != nil {
		return nil, err
	}
	return &db, nil
}

// ListDatabases lists all databases in a project.
func (c *Client) ListDatabases(ctx context.Context, projectID string) ([]*Database, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/databases", nil)
	if err != nil {
		return nil, err
	}
	var dbs []*Database
	if err := c.do(httpReq, &dbs); err != nil {
		return nil, err
	}
	return dbs, nil
}

// DeleteDatabase deletes a database by ID.
func (c *Client) DeleteDatabase(ctx context.Context, id string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/databases/"+id, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// ListBackups lists all backups for a database.
func (c *Client) ListBackups(ctx context.Context, dbID string) ([]DatabaseBackup, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/databases/"+dbID+"/backups", nil)
	if err != nil {
		return nil, err
	}
	var backups []DatabaseBackup
	if err := c.do(httpReq, &backups); err != nil {
		return nil, err
	}
	return backups, nil
}

// CreateBackup triggers a manual backup for a database.
func (c *Client) CreateBackup(ctx context.Context, dbID string) (*DatabaseBackup, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/databases/"+dbID+"/backups", nil)
	if err != nil {
		return nil, err
	}
	var backup DatabaseBackup
	if err := c.do(httpReq, &backup); err != nil {
		return nil, err
	}
	return &backup, nil
}

// GetBackupConfig retrieves the backup configuration for a database.
func (c *Client) GetBackupConfig(ctx context.Context, dbID string) (*BackupConfig, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/databases/"+dbID+"/backup-config", nil)
	if err != nil {
		return nil, err
	}
	var config BackupConfig
	if err := c.do(httpReq, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// UpdateBackupConfig updates the backup configuration for a database.
func (c *Client) UpdateBackupConfig(ctx context.Context, dbID string, config BackupConfig) error {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/databases/"+dbID+"/backup-config", config)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// RestoreDatabase restores a database from a backup.
func (c *Client) RestoreDatabase(ctx context.Context, dbID string, req RestoreRequest) (*Database, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/databases/"+dbID+"/restore", req)
	if err != nil {
		return nil, err
	}
	var db Database
	if err := c.do(httpReq, &db); err != nil {
		return nil, err
	}
	return &db, nil
}

// AddDomain adds a custom domain to an app.
func (c *Client) AddDomain(ctx context.Context, appID string, domain string) (*Domain, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/apps/"+appID+"/domains", DomainRequest{Domain: domain})
	if err != nil {
		return nil, err
	}
	var d Domain
	if err := c.do(httpReq, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ListDomains lists all custom domains for an app.
func (c *Client) ListDomains(ctx context.Context, appID string) ([]*Domain, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+appID+"/domains", nil)
	if err != nil {
		return nil, err
	}
	var domains []*Domain
	if err := c.do(httpReq, &domains); err != nil {
		return nil, err
	}
	return domains, nil
}

// RemoveDomain removes a custom domain from an app.
func (c *Client) RemoveDomain(ctx context.Context, appID string, domain string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/apps/"+appID+"/domains/"+domain, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// SetConfig sets a config value for an app.
func (c *Client) SetConfig(ctx context.Context, appID, key, value string, isSecret bool) (*AppConfig, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/apps/"+appID+"/config", SetConfigRequest{
		Key:      key,
		Value:    value,
		IsSecret: isSecret,
	})
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := c.do(httpReq, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListConfig lists all config values for an app.
func (c *Client) ListConfig(ctx context.Context, appID string) ([]*AppConfig, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+appID+"/config", nil)
	if err != nil {
		return nil, err
	}
	var configs []*AppConfig
	if err := c.do(httpReq, &configs); err != nil {
		return nil, err
	}
	return configs, nil
}

// UnsetConfig removes a config value from an app.
func (c *Client) UnsetConfig(ctx context.Context, appID, key string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/apps/"+appID+"/config/"+key, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// SetupWebhook configures a GitHub webhook for auto-deploy on an app.
func (c *Client) SetupWebhook(ctx context.Context, appID string, req SetupWebhookRequest) (*AppWebhook, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/apps/"+appID+"/webhook", req)
	if err != nil {
		return nil, err
	}
	var wh AppWebhook
	if err := c.do(httpReq, &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// GetWebhook retrieves the webhook configuration for an app.
func (c *Client) GetWebhook(ctx context.Context, appID string) (*AppWebhook, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+appID+"/webhook", nil)
	if err != nil {
		return nil, err
	}
	var wh AppWebhook
	if err := c.do(httpReq, &wh); err != nil {
		return nil, err
	}
	return &wh, nil
}

// RemoveWebhook removes the webhook configuration for an app.
func (c *Client) RemoveWebhook(ctx context.Context, appID string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/apps/"+appID+"/webhook", nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// --- Deployment methods ---

// ListDeployments lists deployment history for an app.
func (c *Client) ListDeployments(ctx context.Context, appID string) ([]*Deployment, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+appID+"/deployments", nil)
	if err != nil {
		return nil, err
	}
	var deployments []*Deployment
	if err := c.do(httpReq, &deployments); err != nil {
		return nil, err
	}
	return deployments, nil
}

// GetDeployment retrieves a single deployment by ID.
func (c *Client) GetDeployment(ctx context.Context, appID, deploymentID string) (*Deployment, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+appID+"/deployments/"+deploymentID, nil)
	if err != nil {
		return nil, err
	}
	var deployment Deployment
	if err := c.do(httpReq, &deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

// --- Organization methods ---

// ListOrgs lists all organizations the authenticated user belongs to.
// CreateOrg creates a new organization (the caller becomes its owner).
func (c *Client) CreateOrg(ctx context.Context, name, displayName string) (*Organization, error) {
	body := map[string]string{"name": name, "display_name": displayName}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/orgs", body)
	if err != nil {
		return nil, err
	}
	var org Organization
	if err := c.do(httpReq, &org); err != nil {
		return nil, err
	}
	return &org, nil
}

func (c *Client) ListOrgs(ctx context.Context) ([]*Organization, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/orgs", nil)
	if err != nil {
		return nil, err
	}
	var orgs []*Organization
	if err := c.do(httpReq, &orgs); err != nil {
		return nil, err
	}
	return orgs, nil
}

// GetOrg retrieves an organization by ID.
func (c *Client) GetOrg(ctx context.Context, id string) (*Organization, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/orgs/"+id, nil)
	if err != nil {
		return nil, err
	}
	var org Organization
	if err := c.do(httpReq, &org); err != nil {
		return nil, err
	}
	return &org, nil
}

// ListOrgMembers lists all members of an organization.
func (c *Client) ListOrgMembers(ctx context.Context, orgID string) ([]*OrgMember, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/orgs/"+orgID+"/members", nil)
	if err != nil {
		return nil, err
	}
	var members []*OrgMember
	if err := c.do(httpReq, &members); err != nil {
		return nil, err
	}
	return members, nil
}

// ProvisionUser creates a new user in an existing organization and mints an
// API key. Admin-only; replaces self-service registration in the internal model.
func (c *Client) ProvisionUser(ctx context.Context, orgID, email, name, role string) (*RegisterResponse, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/orgs/"+orgID+"/users", ProvisionUserRequest{
		Email: email,
		Name:  name,
		Role:  role,
	})
	if err != nil {
		return nil, err
	}
	var resp RegisterResponse
	if err := c.do(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// InviteOrgMember invites a user to an organization by email.
func (c *Client) InviteOrgMember(ctx context.Context, orgID, email, role string) (*OrgMember, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/orgs/"+orgID+"/members", InviteOrgMemberRequest{
		Email: email,
		Role:  role,
	})
	if err != nil {
		return nil, err
	}
	var member OrgMember
	if err := c.do(httpReq, &member); err != nil {
		return nil, err
	}
	return &member, nil
}

// UpdateOrgMemberRole updates a member's role in an organization.
func (c *Client) UpdateOrgMemberRole(ctx context.Context, orgID, userID, role string) error {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/orgs/"+orgID+"/members/"+userID, UpdateOrgMemberRoleRequest{
		Role: role,
	})
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// RemoveOrgMember removes a member from an organization.
func (c *Client) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/orgs/"+orgID+"/members/"+userID, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// --- Service Account methods ---

// CreateServiceAccount creates a new service account in a project.
// CreateTrustBinding creates a per-project OIDC federation trust binding: a repo
// (matched by subject_pattern) on an issuer may assume the given service account.
func (c *Client) CreateTrustBinding(ctx context.Context, projectID string, req CreateTrustBindingRequest) (*TrustBinding, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/federation", req)
	if err != nil {
		return nil, err
	}
	var binding TrustBinding
	if err := c.do(httpReq, &binding); err != nil {
		return nil, err
	}
	return &binding, nil
}

// ListTrustBindings lists the OIDC federation trust bindings in a project.
func (c *Client) ListTrustBindings(ctx context.Context, projectID string) ([]*TrustBinding, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/federation", nil)
	if err != nil {
		return nil, err
	}
	var bindings []*TrustBinding
	if err := c.do(httpReq, &bindings); err != nil {
		return nil, err
	}
	return bindings, nil
}

// DeleteTrustBinding deletes an OIDC federation trust binding in a project.
func (c *Client) DeleteTrustBinding(ctx context.Context, projectID, bindingID string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/projects/"+projectID+"/federation/"+bindingID, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

func (c *Client) CreateServiceAccount(ctx context.Context, projectID string, req CreateServiceAccountRequest) (*ServiceAccount, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/service-accounts", req)
	if err != nil {
		return nil, err
	}
	var sa ServiceAccount
	if err := c.do(httpReq, &sa); err != nil {
		return nil, err
	}
	return &sa, nil
}

// ListServiceAccounts lists all service accounts in a project.
func (c *Client) ListServiceAccounts(ctx context.Context, projectID string) ([]*ServiceAccount, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/service-accounts", nil)
	if err != nil {
		return nil, err
	}
	var accounts []*ServiceAccount
	if err := c.do(httpReq, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

// DeleteServiceAccount deletes a service account by ID.
func (c *Client) DeleteServiceAccount(ctx context.Context, id string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/service-accounts/"+id, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// CreateServiceAccountKey creates a new key for a service account.
func (c *Client) CreateServiceAccountKey(ctx context.Context, saID string) (*ServiceAccountKey, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/service-accounts/"+saID+"/keys", nil)
	if err != nil {
		return nil, err
	}
	var key ServiceAccountKey
	if err := c.do(httpReq, &key); err != nil {
		return nil, err
	}
	return &key, nil
}

// ListServiceAccountKeys lists all keys for a service account.
func (c *Client) ListServiceAccountKeys(ctx context.Context, saID string) ([]*ServiceAccountKey, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/service-accounts/"+saID+"/keys", nil)
	if err != nil {
		return nil, err
	}
	var keys []*ServiceAccountKey
	if err := c.do(httpReq, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteServiceAccountKey deletes a service account key.
func (c *Client) DeleteServiceAccountKey(ctx context.Context, saID, keyID string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/service-accounts/"+saID+"/keys/"+keyID, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// --- IAM Binding methods ---

// SetIAMBinding creates an IAM binding on a project.
func (c *Client) SetIAMBinding(ctx context.Context, projectID string, req SetIAMBindingRequest) (*IAMBinding, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/iam", req)
	if err != nil {
		return nil, err
	}
	var binding IAMBinding
	if err := c.do(httpReq, &binding); err != nil {
		return nil, err
	}
	return &binding, nil
}

// ListIAMBindings lists all IAM bindings for a project.
func (c *Client) ListIAMBindings(ctx context.Context, projectID string) ([]*IAMBinding, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/iam", nil)
	if err != nil {
		return nil, err
	}
	var bindings []*IAMBinding
	if err := c.do(httpReq, &bindings); err != nil {
		return nil, err
	}
	return bindings, nil
}

// RemoveIAMBinding removes an IAM binding from a project.
func (c *Client) RemoveIAMBinding(ctx context.Context, projectID, bindingID string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/projects/"+projectID+"/iam/"+bindingID, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

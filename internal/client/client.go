package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// FKECredentials fetches the cluster connection facts for a kubeconfig context
// scoped to the project (GET /projects/{id}/fke/credentials). Returns an error
// matching client.ErrNotFound when the API predates the endpoint (404), letting
// the CLI fall back to embedded constants.
func (c *Client) FKECredentials(ctx context.Context, projectID string) (*ClusterCredentials, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/fke/credentials", nil)
	if err != nil {
		return nil, err
	}
	var creds ClusterCredentials
	if err := c.do(httpReq, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// ClusterInfo fetches the project-independent cluster connection facts (apiserver
// URL + CA) for assembling a cluster-admin kubeconfig — the staff FKE path, which
// is not project-scoped.
func (c *Client) ClusterInfo(ctx context.Context) (*ClusterInfo, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/cluster-info", nil)
	if err != nil {
		return nil, err
	}
	var info ClusterInfo
	if err := c.do(httpReq, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// FKEToken mints a short-lived, namespace-scoped Kubernetes token bound to the
// project's ServiceAccount (POST /projects/{id}/fke/token). kubectl's exec plugin
// calls this transparently.
func (c *Client) FKEToken(ctx context.Context, projectID string) (*ClusterToken, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/fke/token", nil)
	if err != nil {
		return nil, err
	}
	var tok ClusterToken
	if err := c.do(httpReq, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
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

// RegistryRepository is one image repository visible to a project, with the
// <org_short_id>/<project>/ prefix stripped for display.
type RegistryRepository struct {
	Name string `json:"name"`
}

// RegistryTagList is the set of image tags for one repository.
type RegistryTagList struct {
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
}

// RegistryVulnerabilities is a CVE severity roll-up for one image, from zot's
// search-extension Trivy scanner. Nil/absent when CVE scanning is not enabled.
type RegistryVulnerabilities struct {
	MaxSeverity string `json:"max_severity"`
	Total       int    `json:"total"`
	Critical    int    `json:"critical"`
	High        int    `json:"high"`
	Medium      int    `json:"medium"`
	Low         int    `json:"low"`
	Unknown     int    `json:"unknown"`
}

// RegistryImage is one tagged image with metadata from the zot search extension.
// Size/Digest/PushedAt are zero when the search extension is unavailable.
type RegistryImage struct {
	Tag             string                   `json:"tag"`
	Digest          string                   `json:"digest,omitempty"`
	Size            int64                    `json:"size,omitempty"`
	PushedAt        *time.Time               `json:"pushed_at,omitempty"`
	Vulnerabilities *RegistryVulnerabilities `json:"vulnerabilities,omitempty"`
}

// RegistryImageList is the enriched set of images for one repository.
type RegistryImageList struct {
	Repository string          `json:"repository"`
	Images     []RegistryImage `json:"images"`
}

// RegistryRetentionPolicy is an auto-delete rule for a project's registry repos.
// An empty Repo is the project-wide default. KeepLast keeps the newest N tags;
// MaxAgeDays deletes tags older than N days (newest KeepLast always protected).
type RegistryRetentionPolicy struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Repo       string    `json:"repo"`
	KeepLast   int       `json:"keep_last"`
	MaxAgeDays int       `json:"max_age_days"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SetRetentionPolicyRequest upserts a retention policy for (project, repo).
type SetRetentionPolicyRequest struct {
	Repo       string `json:"repo"`
	KeepLast   int    `json:"keep_last"`
	MaxAgeDays int    `json:"max_age_days"`
	Enabled    bool   `json:"enabled"`
}

// RegistryRepoVisibility is a per-repository public/private setting (ADR-013 S4).
// A repo with Public = true is anonymously pullable; absence of a record means
// private. Repo is the project-relative name.
type RegistryRepoVisibility struct {
	ProjectID string    `json:"project_id"`
	Repo      string    `json:"repo"`
	Public    bool      `json:"public"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SetRegistryVisibilityRequest upserts a repository's visibility for (project, repo).
type SetRegistryVisibilityRequest struct {
	Repo   string `json:"repo"`
	Public bool   `json:"public"`
}

// RetentionPreviewItem is one tag a retention policy would delete.
type RetentionPreviewItem struct {
	Repo     string     `json:"repo"`
	Tag      string     `json:"tag"`
	Digest   string     `json:"digest,omitempty"`
	Reason   string     `json:"reason"`
	PushedAt *time.Time `json:"pushed_at,omitempty"`
}

// RetentionPreview is the dry-run (or applied) set of retention deletions.
type RetentionPreview struct {
	Items []RetentionPreviewItem `json:"items"`
}

// ListRegistryRepositories lists a project's registry repositories.
func (c *Client) ListRegistryRepositories(ctx context.Context, projectID string) ([]RegistryRepository, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/repositories", nil)
	if err != nil {
		return nil, err
	}
	var repos []RegistryRepository
	if err := c.do(httpReq, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// ListRegistryTags lists the tags of one repository (project-relative name).
func (c *Client) ListRegistryTags(ctx context.Context, projectID, repo string) (*RegistryTagList, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet,
		"/api/v1/projects/"+projectID+"/repositories/tags?repo="+url.QueryEscape(repo), nil)
	if err != nil {
		return nil, err
	}
	var tags RegistryTagList
	if err := c.do(httpReq, &tags); err != nil {
		return nil, err
	}
	return &tags, nil
}

// DeleteRegistryTag deletes one tag from a repository (project-relative name).
func (c *Client) DeleteRegistryTag(ctx context.Context, projectID, repo, tag string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete,
		"/api/v1/projects/"+projectID+"/repositories/tags?repo="+url.QueryEscape(repo)+"&tag="+url.QueryEscape(tag), nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// ListRetentionPolicies lists a project's registry retention policies.
func (c *Client) ListRetentionPolicies(ctx context.Context, projectID string) ([]*RegistryRetentionPolicy, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/repositories/retention", nil)
	if err != nil {
		return nil, err
	}
	var policies []*RegistryRetentionPolicy
	if err := c.do(httpReq, &policies); err != nil {
		return nil, err
	}
	return policies, nil
}

// SetRetentionPolicy upserts a retention policy (empty Repo = project default).
func (c *Client) SetRetentionPolicy(ctx context.Context, projectID string, req SetRetentionPolicyRequest) (*RegistryRetentionPolicy, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/projects/"+projectID+"/repositories/retention", req)
	if err != nil {
		return nil, err
	}
	var policy RegistryRetentionPolicy
	if err := c.do(httpReq, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// DeleteRetentionPolicy removes a retention policy (empty repo = project default).
func (c *Client) DeleteRetentionPolicy(ctx context.Context, projectID, repo string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete,
		"/api/v1/projects/"+projectID+"/repositories/retention?repo="+url.QueryEscape(repo), nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// PreviewRetention dry-runs the project's retention policies, returning the
// tags they would delete right now.
func (c *Client) PreviewRetention(ctx context.Context, projectID string) (*RetentionPreview, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/repositories/retention/preview", nil)
	if err != nil {
		return nil, err
	}
	var preview RetentionPreview
	if err := c.do(httpReq, &preview); err != nil {
		return nil, err
	}
	return &preview, nil
}

// ApplyRetention enforces the project's retention policies now, deleting the
// selected tags and returning them.
func (c *Client) ApplyRetention(ctx context.Context, projectID string) (*RetentionPreview, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/repositories/retention/apply", nil)
	if err != nil {
		return nil, err
	}
	var preview RetentionPreview
	if err := c.do(httpReq, &preview); err != nil {
		return nil, err
	}
	return &preview, nil
}

// ListRegistryVisibility lists a project's per-repo visibility records. A repo
// with no record is private (the default).
func (c *Client) ListRegistryVisibility(ctx context.Context, projectID string) ([]*RegistryRepoVisibility, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/repositories/visibility", nil)
	if err != nil {
		return nil, err
	}
	var records []*RegistryRepoVisibility
	if err := c.do(httpReq, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// SetRegistryVisibility sets one repository's public/private visibility.
func (c *Client) SetRegistryVisibility(ctx context.Context, projectID string, req SetRegistryVisibilityRequest) (*RegistryRepoVisibility, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/projects/"+projectID+"/repositories/visibility", req)
	if err != nil {
		return nil, err
	}
	var record RegistryRepoVisibility
	if err := c.do(httpReq, &record); err != nil {
		return nil, err
	}
	return &record, nil
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

// UpdateProjectQuota sets a project's operator-only resource caps; only the
// non-nil caps are changed.
func (c *Client) UpdateProjectQuota(ctx context.Context, id string, maxCPU, maxMemory *string, maxPods *int, maxStorage *string) (*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/projects/"+id, UpdateProjectRequest{MaxCPU: maxCPU, MaxMemory: maxMemory, MaxPods: maxPods, MaxStorage: maxStorage})
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
// (resource_type, resource_id, actor, limit, offset).
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

// MoveProjectResult is the response from re-homing a project to its org-prefixed
// namespace: the updated project plus any per-app redeploy warnings.
type MoveProjectResult struct {
	Project  *Project `json:"project"`
	Warnings []string `json:"warnings,omitempty"`
}

// MoveProject re-homes a project into its canonical org-prefixed namespace
// (<org short id>-<name>). force proceeds past the stateful-resource guard;
// database/PVC data is not migrated and must be handled separately.
func (c *Client) MoveProject(ctx context.Context, id string, force bool) (*MoveProjectResult, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+id+"/move", map[string]bool{"force": force})
	if err != nil {
		return nil, err
	}
	var res MoveProjectResult
	if err := c.do(httpReq, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// UpdateProjectDisplayName changes a project's mutable, cosmetic display name
// (ADR-036). The frozen name — which anchors the k8s namespace and registry path —
// is untouched, so this is a plain label change with no cluster impact.
func (c *Client) UpdateProjectDisplayName(ctx context.Context, id, displayName string) (*Project, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/projects/"+id, UpdateProjectRequest{DisplayName: displayName})
	if err != nil {
		return nil, err
	}
	var proj Project
	if err := c.do(httpReq, &proj); err != nil {
		return nil, err
	}
	return &proj, nil
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

// UpdateAppDisplayName changes an app's mutable, cosmetic display name (ADR-036).
// The frozen name — which names the k8s resources and the URL — is untouched, so
// this is a plain label change with no downtime or redeploy.
func (c *Client) UpdateAppDisplayName(ctx context.Context, id, displayName string) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/apps/"+id, UpdateAppRequest{DisplayName: displayName})
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// UpdateAppURLSlug sets or clears an app's optional vanity host override (ADR-040).
// An empty slug clears it, reverting the host to the derived label; a non-empty slug
// makes the app reachable at <slug>.app.<platform_domain>. Always-on mode only.
func (c *Client) UpdateAppURLSlug(ctx context.Context, id, slug string) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/apps/"+id, UpdateAppRequest{URLSlug: &slug})
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// SwitchMode migrates an app between hosting modes ("always-on"/"serverless").
func (c *Client) SwitchMode(ctx context.Context, id, mode string) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/apps/"+id+"/mode", SwitchModeRequest{Mode: mode})
	if err != nil {
		return nil, err
	}
	var app App
	if err := c.do(httpReq, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// UpdateAppStorage grows an app's persistent volume (grow-only, always-on mode).
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

// UpdateAppCommand changes an app's container entrypoint override (command),
// arguments (args), and/or release command. Each is optional: a nil pointer
// leaves the value untouched, a non-nil pointer (including an empty slice)
// replaces it — an empty slice clears the override back to the image defaults
// (or drops the release phase).
func (c *Client) UpdateAppCommand(ctx context.Context, id string, command, args, releaseCommand *[]string) (*App, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/apps/"+id+"/command", UpdateCommandRequest{Command: command, Args: args, ReleaseCommand: releaseCommand})
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

// GetDatabaseConnection retrieves a database's live connection info (incl. the
// real CNPG password) for the `db connect` tunnel path.
func (c *Client) GetDatabaseConnection(ctx context.Context, id string) (*DatabaseConnection, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/databases/"+id+"/connection", nil)
	if err != nil {
		return nil, err
	}
	var conn DatabaseConnection
	if err := c.do(httpReq, &conn); err != nil {
		return nil, err
	}
	return &conn, nil
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

// CreateBucket provisions a managed object-storage bucket in a project (ADR-039).
// The response carries the one-time secret access key.
func (c *Client) CreateBucket(ctx context.Context, projectID string, req CreateBucketRequest) (*Bucket, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/buckets", req)
	if err != nil {
		return nil, err
	}
	var b Bucket
	if err := c.do(httpReq, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBuckets lists all buckets in a project.
func (c *Client) ListBuckets(ctx context.Context, projectID string) ([]*Bucket, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/projects/"+projectID+"/buckets", nil)
	if err != nil {
		return nil, err
	}
	var buckets []*Bucket
	if err := c.do(httpReq, &buckets); err != nil {
		return nil, err
	}
	return buckets, nil
}

// GetBucket retrieves a bucket by ID.
func (c *Client) GetBucket(ctx context.Context, id string) (*Bucket, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/buckets/"+id, nil)
	if err != nil {
		return nil, err
	}
	var b Bucket
	if err := c.do(httpReq, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// DeleteBucket deletes a bucket by ID. A non-empty bucket returns a 409 APIError.
func (c *Client) DeleteBucket(ctx context.Context, id string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/buckets/"+id, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// GetBucketCredentials returns a bucket's S3 connection details. The secret is
// only present when a fresh key was minted.
func (c *Client) GetBucketCredentials(ctx context.Context, id string) (*BucketCredentials, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/buckets/"+id+"/credentials", nil)
	if err != nil {
		return nil, err
	}
	var creds BucketCredentials
	if err := c.do(httpReq, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// SetBucketQuota updates a bucket's quotas (bytes / object count; 0 = unlimited).
func (c *Client) SetBucketQuota(ctx context.Context, id string, maxSize, maxObjects int64) (*Bucket, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/buckets/"+id+"/quota", SetBucketQuotaRequest{QuotaMaxSize: maxSize, QuotaMaxObjects: maxObjects})
	if err != nil {
		return nil, err
	}
	var b Bucket
	if err := c.do(httpReq, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// SetBucketWebsite toggles static-website serving on a bucket (#342). Enabling
// serves the bucket anonymously over HTTP at the returned WebsiteURL.
func (c *Client) SetBucketWebsite(ctx context.Context, id string, req SetBucketWebsiteRequest) (*Bucket, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/buckets/"+id+"/website", req)
	if err != nil {
		return nil, err
	}
	var b Bucket
	if err := c.do(httpReq, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// SetBucketURLSlug sets (or clears, with "") a bucket website's vanity host
// label; the site moves to <slug>.web.<platform domain>.
func (c *Client) SetBucketURLSlug(ctx context.Context, id, slug string) (*Bucket, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/buckets/"+id+"/url-slug", SetBucketURLSlugRequest{URLSlug: slug})
	if err != nil {
		return nil, err
	}
	var b Bucket
	if err := c.do(httpReq, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// CreateBucketKey mints a scoped S3 access key for a bucket. The response carries
// the one-time secret access key.
func (c *Client) CreateBucketKey(ctx context.Context, bucketID string, req CreateBucketKeyRequest) (*BucketKey, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/buckets/"+bucketID+"/keys", req)
	if err != nil {
		return nil, err
	}
	var k BucketKey
	if err := c.do(httpReq, &k); err != nil {
		return nil, err
	}
	return &k, nil
}

// ListBucketKeys lists a bucket's scoped keys (never the secret).
func (c *Client) ListBucketKeys(ctx context.Context, bucketID string) ([]*BucketKey, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/buckets/"+bucketID+"/keys", nil)
	if err != nil {
		return nil, err
	}
	var keys []*BucketKey
	if err := c.do(httpReq, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteBucketKey revokes a scoped access key.
func (c *Client) DeleteBucketKey(ctx context.Context, bucketID, accessKeyID string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/buckets/"+bucketID+"/keys/"+accessKeyID, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// UpdateBucketKeyPermissions changes a scoped key's read/write/owner grants.
func (c *Client) UpdateBucketKeyPermissions(ctx context.Context, bucketID, accessKeyID string, req UpdateBucketKeyPermissionsRequest) (*BucketKey, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/buckets/"+bucketID+"/keys/"+accessKeyID, req)
	if err != nil {
		return nil, err
	}
	var k BucketKey
	if err := c.do(httpReq, &k); err != nil {
		return nil, err
	}
	return &k, nil
}

// ListBucketObjects lists a bucket's objects under prefix, grouping folders at
// "/" (in-browser object browser, #268). An empty prefix lists the root.
func (c *Client) ListBucketObjects(ctx context.Context, bucketID, prefix string) (*ObjectListing, error) {
	path := "/api/v1/buckets/" + bucketID + "/objects?delimiter=" + url.QueryEscape("/")
	if prefix != "" {
		path += "&prefix=" + url.QueryEscape(prefix)
	}
	httpReq, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var listing ObjectListing
	if err := c.do(httpReq, &listing); err != nil {
		return nil, err
	}
	return &listing, nil
}

// PresignBucketObject mints a presigned S3 URL for a GET (download) or PUT
// (upload) so the browser transfers bytes straight to the object store (#268).
func (c *Client) PresignBucketObject(ctx context.Context, bucketID string, req PresignObjectRequest) (*PresignResponse, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/buckets/"+bucketID+"/objects/presign", req)
	if err != nil {
		return nil, err
	}
	var res PresignResponse
	if err := c.do(httpReq, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// DeleteBucketObject deletes a single object from a bucket (#268).
func (c *Client) DeleteBucketObject(ctx context.Context, bucketID, key string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/buckets/"+bucketID+"/objects?key="+url.QueryEscape(key), nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// BindAppBucket binds a bucket to an app, injecting its S3_*/AWS_* credentials
// into the app's pod (#264). readOnly requests a read-only scoped key.
func (c *Client) BindAppBucket(ctx context.Context, appID, bucketID string, readOnly bool) (*AppBucketBinding, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/apps/"+appID+"/buckets", BindBucketRequest{BucketID: bucketID, ReadOnly: readOnly})
	if err != nil {
		return nil, err
	}
	var b AppBucketBinding
	if err := c.do(httpReq, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListAppBuckets lists an app's bucket bindings (never the secret).
func (c *Client) ListAppBuckets(ctx context.Context, appID string) ([]*AppBucketBinding, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/apps/"+appID+"/buckets", nil)
	if err != nil {
		return nil, err
	}
	var bindings []*AppBucketBinding
	if err := c.do(httpReq, &bindings); err != nil {
		return nil, err
	}
	return bindings, nil
}

// UnbindAppBucket removes an app ⇄ bucket binding, dropping the injected creds.
func (c *Client) UnbindAppBucket(ctx context.Context, appID, bucketID string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/apps/"+appID+"/buckets/"+bucketID, nil)
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

// DeleteBackup removes a single managed backup (its Backup CR + object-store
// artifact). The API refuses to delete the backup anchoring the recovery window.
func (c *Client) DeleteBackup(ctx context.Context, dbID, name string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/databases/"+dbID+"/backups/"+name, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
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

// GetBackupDestination retrieves a database's external (BYOB) backup destination.
func (c *Client) GetBackupDestination(ctx context.Context, dbID string) (*BackupDestination, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/databases/"+dbID+"/backup-destination", nil)
	if err != nil {
		return nil, err
	}
	var dest BackupDestination
	if err := c.do(httpReq, &dest); err != nil {
		return nil, err
	}
	return &dest, nil
}

// SetBackupDestination configures (or replaces) a database's external backup
// destination — the customer's own bucket, keyless.
func (c *Client) SetBackupDestination(ctx context.Context, dbID string, dest BackupDestination) (*BackupDestination, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/databases/"+dbID+"/backup-destination", dest)
	if err != nil {
		return nil, err
	}
	var out BackupDestination
	if err := c.do(httpReq, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteBackupDestination removes a database's external backup destination.
func (c *Client) DeleteBackupDestination(ctx context.Context, dbID string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/databases/"+dbID+"/backup-destination", nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

// RunBackupDestination starts an on-demand external backup (pg_dump → the
// customer bucket). The backup runs asynchronously as a k8s Job.
func (c *Client) RunBackupDestination(ctx context.Context, dbID string) (*BackupDestinationRun, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/databases/"+dbID+"/backup-destination/sync", nil)
	if err != nil {
		return nil, err
	}
	var run BackupDestinationRun
	if err := c.do(httpReq, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// RestoreBackupDestination restores a database from a dump in the customer bucket
// (pg_restore). object names the dump; empty restores the latest.
func (c *Client) RestoreBackupDestination(ctx context.Context, dbID, object string) (*BackupDestinationRun, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/databases/"+dbID+"/backup-destination/restore", map[string]string{"object": object})
	if err != nil {
		return nil, err
	}
	var run BackupDestinationRun
	if err := c.do(httpReq, &run); err != nil {
		return nil, err
	}
	return &run, nil
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

// AddDomain adds a custom domain to an app. mode selects the attachment behavior
// (ADR-044); an empty mode defaults to "verified".
func (c *Client) AddDomain(ctx context.Context, appID, domain, mode string) (*Domain, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/apps/"+appID+"/domains", DomainRequest{Domain: domain, Mode: mode})
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

// VerifyDomain re-checks TXT ownership + DNS pointing for a custom domain and
// returns the full verification breakdown plus the records still needed.
func (c *Client) VerifyDomain(ctx context.Context, appID string, domain string) (*DomainVerification, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/apps/"+appID+"/domains/"+domain+"/verify", nil)
	if err != nil {
		return nil, err
	}
	var v DomainVerification
	if err := c.do(httpReq, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// RemoveDomain removes a custom domain from an app.
// AddBucketDomain claims a custom domain for a website bucket (#342).
func (c *Client) AddBucketDomain(ctx context.Context, bucketID, domain string) (*Domain, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/buckets/"+bucketID+"/domains", DomainRequest{Domain: domain})
	if err != nil {
		return nil, err
	}
	var d Domain
	if err := c.do(httpReq, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ListBucketDomains lists a website bucket's custom domains.
func (c *Client) ListBucketDomains(ctx context.Context, bucketID string) ([]*Domain, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/buckets/"+bucketID+"/domains", nil)
	if err != nil {
		return nil, err
	}
	var domains []*Domain
	if err := c.do(httpReq, &domains); err != nil {
		return nil, err
	}
	return domains, nil
}

// VerifyBucketDomain re-checks a website bucket domain's ownership/pointing/cert.
func (c *Client) VerifyBucketDomain(ctx context.Context, bucketID, domain string) (*DomainVerification, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/buckets/"+bucketID+"/domains/"+domain+"/verify", nil)
	if err != nil {
		return nil, err
	}
	var vr DomainVerification
	if err := c.do(httpReq, &vr); err != nil {
		return nil, err
	}
	return &vr, nil
}

// RemoveBucketDomain removes a custom domain from a website bucket.
func (c *Client) RemoveBucketDomain(ctx context.Context, bucketID, domain string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/buckets/"+bucketID+"/domains/"+domain, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

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
// CreateOrg creates a new organization (the caller becomes its owner). shortID
// optionally sets an explicit org id (the platform-org override); empty = an
// opaque random id is assigned server-side.
func (c *Client) CreateOrg(ctx context.Context, name, displayName, shortID string) (*Organization, error) {
	body := map[string]string{"name": name, "display_name": displayName}
	if shortID != "" {
		body["short_id"] = shortID
	}
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

// UpdateOrgFKE toggles an organization's FKE entitlement (kubectl/kubeconfig
// access). Operator-only server-side: a tenant cannot enable it for their org.
func (c *Client) UpdateOrgFKE(ctx context.Context, id string, enabled bool) (*Organization, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/orgs/"+id, UpdateOrgRequest{FKEEnabled: &enabled})
	if err != nil {
		return nil, err
	}
	var org Organization
	if err := c.do(httpReq, &org); err != nil {
		return nil, err
	}
	return &org, nil
}

// UpdateOrgDisplayName changes an organization's mutable, cosmetic display name.
// The frozen name and short_id are untouched. Gated on org-admin server-side.
func (c *Client) UpdateOrgDisplayName(ctx context.Context, id, displayName string) (*Organization, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/orgs/"+id, UpdateOrgRequest{DisplayName: displayName})
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

// UpdateServiceAccountDisplayName changes a service account's mutable, cosmetic
// display name. The frozen name and email are untouched.
func (c *Client) UpdateServiceAccountDisplayName(ctx context.Context, id, displayName string) (*ServiceAccount, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPatch, "/api/v1/service-accounts/"+id, UpdateServiceAccountRequest{DisplayName: displayName})
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

// ListOrgSecrets lists an org's Fogpipe Secrets Manager bundles (key names only).
func (c *Client) ListOrgSecrets(ctx context.Context, orgID string) ([]*OrgSecret, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/orgs/"+orgID+"/secrets", nil)
	if err != nil {
		return nil, err
	}
	var secrets []*OrgSecret
	if err := c.do(httpReq, &secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

// GetOrgSecret retrieves a single bundle. When reveal is true, the decrypted
// values are returned in Data (requires org write permission).
func (c *Client) GetOrgSecret(ctx context.Context, orgID, name string, reveal bool) (*OrgSecret, error) {
	path := "/api/v1/orgs/" + orgID + "/secrets/" + name
	if reveal {
		path += "?reveal=true"
	}
	httpReq, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var secret OrgSecret
	if err := c.do(httpReq, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// CreateOrgSecret creates a new bundle with the given key/value data, mirrored
// into the given target project ids.
func (c *Client) CreateOrgSecret(ctx context.Context, orgID, name string, data map[string]string, targets []string) (*OrgSecret, error) {
	body := map[string]any{"name": name, "data": data, "targets": targets}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/orgs/"+orgID+"/secrets", body)
	if err != nil {
		return nil, err
	}
	var secret OrgSecret
	if err := c.do(httpReq, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// UpdateOrgSecret replaces an existing bundle's data and target projects
// (full-desired-state replace).
func (c *Client) UpdateOrgSecret(ctx context.Context, orgID, name string, data map[string]string, targets []string) (*OrgSecret, error) {
	body := map[string]any{"data": data, "targets": targets}
	httpReq, err := c.newRequest(ctx, http.MethodPut, "/api/v1/orgs/"+orgID+"/secrets/"+name, body)
	if err != nil {
		return nil, err
	}
	var secret OrgSecret
	if err := c.do(httpReq, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// DeleteOrgSecret removes a bundle.
func (c *Client) DeleteOrgSecret(ctx context.Context, orgID, name string) error {
	httpReq, err := c.newRequest(ctx, http.MethodDelete, "/api/v1/orgs/"+orgID+"/secrets/"+name, nil)
	if err != nil {
		return err
	}
	return c.do(httpReq, nil)
}

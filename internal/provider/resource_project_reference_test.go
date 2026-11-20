package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeReferenceAPI is the part of the API an app, a database, a secret and a
// job touch, resolving a project or an organization by name or id at the route
// boundary, as the platform's router does, and answering with ids.
type fakeReferenceAPI struct {
	mu        sync.Mutex
	seq       int
	apps      map[string]*client.App
	databases map[string]*client.Database
	secrets   map[string]*client.Secret
	jobs      map[string]*client.Job
	// writes counts every request that changes what an existing app,
	// database or secret holds. Creates and deletes are not writes.
	writes int
}

// fakeOrgs is every organization the fake API holds, by each spelling it
// resolves.
var fakeOrgs = map[string]client.Organization{
	"acme":   {ID: "org-1", ShortID: "acme1", DisplayName: "acme"},
	"org-1":  {ID: "org-1", ShortID: "acme1", DisplayName: "acme"},
	"globex": {ID: "org-2", ShortID: "globex2", DisplayName: "globex"},
	"org-2":  {ID: "org-2", ShortID: "globex2", DisplayName: "globex"},
}

func startFakeReferenceAPI(t *testing.T) *fakeReferenceAPI {
	t.Helper()
	terraformBinary(t)
	api := &fakeReferenceAPI{
		apps:      map[string]*client.App{},
		databases: map[string]*client.Database{},
		secrets:   map[string]*client.Secret{},
		jobs:      map[string]*client.Job{},
	}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")
	return api
}

func (f *fakeReferenceAPI) wrote(want int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.writes != want {
			return fmt.Errorf("%d write(s) reached the API, want %d", f.writes, want)
		}
		return nil
	}
}

func (f *fakeReferenceAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	missing := func() {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "NOT_FOUND", "message": "not found"}})
	}
	answer := func(status int, v any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	// A project or an organization is resolved by name or id, and whatever
	// follows reads its id.
	scope, scopeID := "", ""
	for prefix, kind := range map[string]string{"/api/v1/projects/": "project", "/api/v1/orgs/": "org"} {
		rest, ok := strings.CutPrefix(r.URL.Path, prefix)
		if !ok {
			continue
		}
		ref, tail, _ := strings.Cut(rest, "/")
		var entity any
		if kind == "project" {
			p, known := fakeProjects[ref]
			entity, scopeID = p, p.ID
			if !known {
				missing()
				return
			}
		} else {
			o, known := fakeOrgs[ref]
			entity, scopeID = o, o.ID
			if !known {
				missing()
				return
			}
		}
		if tail == "" {
			answer(http.StatusOK, entity)
			return
		}
		scope, r.URL.Path = kind, "/"+tail
	}
	kind, id, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	id, sub, _ := strings.Cut(id, "/")
	next := func(prefix string) string {
		f.seq++
		return fmt.Sprintf("%s-%d", prefix, f.seq)
	}

	switch {
	case scope == "project" && r.Method == http.MethodPost && r.URL.Path == "/apps":
		var req client.CreateAppRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		app := &client.App{ID: next("app"), ProjectID: scopeID, Name: req.Name, DisplayName: req.Name,
			Image: req.Image, Type: "web", Port: 8080, Ingress: "internal", Status: "running",
			MinScale: 1, MaxScale: 1, CPULimit: "500m", MemoryLimit: "512Mi",
			CreatedAt: stamp, UpdatedAt: stamp}
		f.apps[app.ID] = app
		answer(http.StatusCreated, app)
	case scope == "project" && r.Method == http.MethodPost && r.URL.Path == "/databases":
		var req client.CreateDatabaseRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		db := &client.Database{ID: next("db"), ProjectID: scopeID, Name: req.Name, DisplayName: req.Name,
			Engine: "postgres", Version: "18", Status: "running", Host: req.Name + "-rw", Port: 5432,
			CPU: "250m", Memory: "512Mi", Storage: "10Gi", Instances: 2, CreatedAt: stamp, UpdatedAt: stamp}
		f.databases[db.ID] = db
		answer(http.StatusCreated, db)
	case scope == "project" && r.Method == http.MethodGet && r.URL.Path == "/apps":
		apps := []*client.App{}
		for _, a := range f.apps {
			if a.ProjectID == scopeID {
				apps = append(apps, a)
			}
		}
		answer(http.StatusOK, apps)
	case scope == "project" && r.Method == http.MethodPost && r.URL.Path == "/jobs":
		var req client.CreateJobRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		job := &client.Job{ID: next("job"), ProjectID: scopeID, Name: req.Name, DisplayName: req.Name,
			Schedule: req.Schedule, Timezone: "UTC", Concurrency: "forbid", MaxRetries: 3, Timeout: 3600,
			KeepRuns: 10, Target: client.JobTargetContainer, CreatedAt: stamp, UpdatedAt: stamp}
		// The app is resolved in the job's project, by id or by name.
		for _, a := range f.apps {
			if a.ProjectID == scopeID && (req.App == a.ID || req.App == a.Name) {
				job.AppID, job.AppName, job.Image = a.ID, a.Name, a.Image
			}
		}
		if req.App != "" && job.AppID == "" {
			missing()
			return
		}
		f.jobs[job.ID] = job
		answer(http.StatusCreated, job)
	case scope != "" && r.Method == http.MethodPost && r.URL.Path == "/secrets":
		var req client.CreateSecretRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sec := &client.Secret{ID: next("sec"), Name: req.Name, CreatedAt: stamp, UpdatedAt: stamp}
		if scope == "project" {
			sec.Scope, sec.ProjectID = "project", scopeID
		} else {
			sec.Scope, sec.OrganizationID = "organization", scopeID
		}
		f.secrets[sec.ID] = sec
		answer(http.StatusCreated, sec)

	case kind == "apps" && f.apps[id] == nil, kind == "databases" && f.databases[id] == nil,
		kind == "secrets" && f.secrets[id] == nil, kind == "jobs" && f.jobs[id] == nil:
		missing()
	case kind == "apps" && sub == "deployments":
		answer(http.StatusOK, []client.Deployment{})
	case r.Method == http.MethodDelete && sub == "" && kind == "databases":
		// A database's delete is accepted and answered with the database.
		db := *f.databases[id]
		db.Status = "deleting"
		delete(f.databases, id)
		answer(http.StatusAccepted, db)
	case r.Method == http.MethodDelete && sub == "":
		delete(f.apps, id)
		delete(f.secrets, id)
		delete(f.jobs, id)
		w.WriteHeader(http.StatusNoContent)
	case r.Method != http.MethodGet:
		f.writes++
		fallthrough
	default:
		switch kind {
		case "apps":
			answer(http.StatusOK, f.apps[id])
		case "databases":
			answer(http.StatusOK, f.databases[id])
		case "secrets":
			answer(http.StatusOK, f.secrets[id])
		case "jobs":
			answer(http.StatusOK, f.jobs[id])
		default:
			missing()
		}
	}
}

const projectApp = `
resource "fpcloud_app" "web" {
  project = %q
  name    = "web"
  image   = "ghcr.io/acme/web:v1"
}`

// `project` takes the project by name or id, as the API's routes do, and holds
// what was written: the API answers with the id, and writing that back made
// every apply of a name inconsistent with its own plan, leaving the app
// tainted and replaced on every apply. Every applying step here fails on a
// plan that is not empty after it.
func TestApp_ProjectByName(t *testing.T) {
	api := startFakeReferenceAPI(t)
	name := "fpcloud_app.web"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(projectApp, "demo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "demo"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
				),
			},
			{
				// The same project by its id: an update in place that
				// writes nothing.
				Config: fmt.Sprintf(projectApp, "proj-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "proj-1"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
					resource.TestCheckResourceAttr(name, "id", "app-1"),
					api.wrote(0),
				),
			},
			{
				// A different project is a different app.
				Config: fmt.Sprintf(projectApp, "other"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "other"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-2"),
				),
			},
			{
				// A project that resolves to nothing fails the plan, before
				// anything is replaced.
				Config:      fmt.Sprintf(projectApp, "nowhere"),
				ExpectError: regexp.MustCompile(`Error resolving project`),
			},
		},
	})
}

// A state written before `project` was kept as written held the id the API
// answered in `project_id`, at schema version 2. It upgrades to that id in
// both, so the configuration that release applied, rewritten onto `project`,
// plans no change; naming the project by name after it is an update in place
// that writes nothing.
func TestApp_StateFromBeforeProjectWasKept(t *testing.T) {
	api := startFakeReferenceAPI(t)
	name := "fpcloud_app.web"
	resource.UnitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ProtoV6ProviderFactories: legacyProvider("fpcloud_app", 2, legacyReference{"project", "project_id", "project_id"}),
				Config: `
resource "fpcloud_app" "web" {
  project_id = "proj-1"
  name       = "web"
  image      = "ghcr.io/acme/web:v1"
}`,
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   fmt.Sprintf(projectApp, "proj-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "proj-1"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
				),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   fmt.Sprintf(projectApp, "demo"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "demo"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
					resource.TestCheckResourceAttr(name, "id", "app-1"),
					api.wrote(0),
				),
			},
		},
	})
}

const projectDatabase = `
resource "fpcloud_database" "main" {
  project = %q
  name    = "maindb"
}`

// A database replaced because its project was spelled by name is a database
// whose data is gone, so the same holds for it as for an app.
func TestDatabase_ProjectByName(t *testing.T) {
	api := startFakeReferenceAPI(t)
	name := "fpcloud_database.main"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(projectDatabase, "demo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "demo"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
				),
			},
			{
				// The update in place sends nothing: a database's update is
				// a whole-spec PATCH, and there is nothing to change.
				Config: fmt.Sprintf(projectDatabase, "proj-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "proj-1"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
					resource.TestCheckResourceAttr(name, "id", "db-1"),
					api.wrote(0),
				),
			},
			{
				Config: fmt.Sprintf(projectDatabase, "other"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(name, "project_id", "proj-2"),
			},
			{
				Config:      fmt.Sprintf(projectDatabase, "nowhere"),
				ExpectError: regexp.MustCompile(`Error resolving project`),
			},
		},
	})
}

// A database's state from schema version 0 upgrades as an app's does.
func TestDatabase_StateFromBeforeProjectWasKept(t *testing.T) {
	api := startFakeReferenceAPI(t)
	name := "fpcloud_database.main"
	resource.UnitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ProtoV6ProviderFactories: legacyProvider("fpcloud_database", 0, legacyReference{"project", "project_id", "project_id"}),
				Config: `
resource "fpcloud_database" "main" {
  project_id = "proj-1"
  name       = "maindb"
}`,
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   fmt.Sprintf(projectDatabase, "proj-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   fmt.Sprintf(projectDatabase, "demo"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "demo"),
					resource.TestCheckResourceAttr(name, "id", "db-1"),
					api.wrote(0),
				),
			},
		},
	})
}

const orgSecret = `
resource "fpcloud_secret" "token" {
  org   = %q
  name  = "token"
  value = "s3cret"
}`

// An organization is a reference like a project: `org` holds what was written
// beside the computed `organization_id`. Another spelling of the organization
// rewrites nothing, which for a secret matters twice over: writing the value
// again rolls every app that references it.
func TestSecret_OrgByName(t *testing.T) {
	api := startFakeReferenceAPI(t)
	name := "fpcloud_secret.token"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(orgSecret, "acme"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "org", "acme"),
					resource.TestCheckResourceAttr(name, "organization_id", "org-1"),
					resource.TestCheckNoResourceAttr(name, "project"),
					resource.TestCheckNoResourceAttr(name, "project_id"),
				),
			},
			{
				Config: fmt.Sprintf(orgSecret, "org-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "org", "org-1"),
					resource.TestCheckResourceAttr(name, "organization_id", "org-1"),
					api.wrote(0),
				),
			},
			{
				Config: fmt.Sprintf(orgSecret, "globex"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(name, "organization_id", "org-2"),
			},
			{
				// Moving the secret into a project is a new secret too.
				Config: `
resource "fpcloud_secret" "token" {
  project = "demo"
  name    = "token"
  value   = "s3cret"
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
					resource.TestCheckNoResourceAttr(name, "organization_id"),
				),
			},
		},
	})
}

// A secret's state from schema version 0 held its organization in
// `organization_id`, the name the computed id now has.
func TestSecret_StateFromBeforeOrgWasKept(t *testing.T) {
	api := startFakeReferenceAPI(t)
	name := "fpcloud_secret.token"
	resource.UnitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ProtoV6ProviderFactories: legacyProvider("fpcloud_secret", 0,
					legacyReference{"project", "project_id", "project_id"},
					legacyReference{"org", "organization_id", "organization_id"}),
				Config: `
resource "fpcloud_secret" "token" {
  organization_id = "org-1"
  name            = "token"
  value           = "s3cret"
}`,
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   fmt.Sprintf(orgSecret, "org-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   fmt.Sprintf(orgSecret, "acme"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "org", "acme"),
					resource.TestCheckResourceAttr(name, "organization_id", "org-1"),
					api.wrote(0),
				),
			},
		},
	})
}

// A job's `app` is a reference like its `project`: kept as written beside the
// `app_id` it resolved to in the job's project, so naming the app by its id
// after its name is an update that writes nothing, and only a different app
// replaces the job.
func TestJob_AppByName(t *testing.T) {
	api := startFakeReferenceAPI(t)
	config := func(app string) string {
		return `
resource "fpcloud_app" "web" {
  project = "demo"
  name    = "web"
  image   = "ghcr.io/acme/web:v1"
}

resource "fpcloud_app" "worker" {
  project = "demo"
  name    = "worker"
  image   = "ghcr.io/acme/web:v1"
}

resource "fpcloud_job" "nightly" {
  project  = "demo"
  name     = "nightly"
  schedule = "0 3 * * *"
  app      = ` + app + `
}`
	}
	name := "fpcloud_job.nightly"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("fpcloud_app.web.name"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "app", "web"),
					resource.TestCheckResourceAttrPair(name, "app_id", "fpcloud_app.web", "id"),
				),
			},
			{
				Config: config("fpcloud_app.web.id"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(name, "app", "fpcloud_app.web", "id"),
					resource.TestCheckResourceAttrPair(name, "app_id", "fpcloud_app.web", "id"),
					api.wrote(0),
				),
			},
			{
				Config: config(`"worker"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttrPair(name, "app_id", "fpcloud_app.worker", "id"),
			},
			{
				Config:      config(`"nowhere"`),
				ExpectError: regexp.MustCompile(`Error resolving app`),
			},
		},
	})
}

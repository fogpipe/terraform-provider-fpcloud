package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
)

// accSweepAge is how old a project in the acceptance org has to be before a
// run reclaims it. Every test runs under `go test -timeout 30m`, so nothing a
// live run owns — this one or a concurrent one — can be this old; a project
// that is belongs to a run that was killed before its teardown.
const accSweepAge = 2 * time.Hour

// TestMain reclaims what an earlier run left behind before this one starts
// (fogpipe/cloud-workspace#70). The framework destroys a test's resources in
// its teardown, and a cancelled workflow or a ^C is a killed process that never
// reaches it — so each leaked project stays, counting against the org's
// MAX_PROJECTS_PER_ORG until the suite cannot create one at all.
//
// The sweep selects positively: the projects of the org FPCLOUD_ACC_SWEEP_ORG
// names, older than accSweepAge. It never reads a name as a reason to delete.
// Unset, nothing is swept — a personal key pointed at a real org must not lose
// its projects to a test helper — and set to an org the key cannot see, the
// run stops rather than proceed into a suite that will soon be refused.
func TestMain(m *testing.M) {
	if os.Getenv("TF_ACC") != "" {
		if err := sweepAccOrg(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "acceptance sweep:", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func sweepAccOrg(ctx context.Context) error {
	orgName := os.Getenv("FPCLOUD_ACC_SWEEP_ORG")
	if orgName == "" {
		fmt.Fprintln(os.Stderr, "acceptance sweep: FPCLOUD_ACC_SWEEP_ORG unset, not reclaiming leaked projects")
		return nil
	}
	c := testAccClient()
	orgs, err := c.ListOrgs(ctx)
	if err != nil {
		return fmt.Errorf("list orgs: %w", err)
	}
	var org *client.Organization
	for _, o := range orgs {
		if o.ShortID == orgName || strings.EqualFold(o.DisplayName, orgName) {
			org = o
		}
	}
	if org == nil {
		return fmt.Errorf("the API key does not belong to org %q; refusing to sweep or to run", orgName)
	}
	projects, err := c.ListProjectsInOrg(ctx, org.ID)
	if err != nil {
		return fmt.Errorf("list projects in %s: %w", orgName, err)
	}
	cutoff := time.Now().Add(-accSweepAge)
	for _, p := range projects {
		if p.Status == "deleting" || !p.CreatedAt.Before(cutoff) {
			continue
		}
		if _, err := c.DeleteProject(ctx, p.ID); err != nil {
			return fmt.Errorf("delete leaked project %s (created %s): %w", p.Name, p.CreatedAt.Format(time.RFC3339), err)
		}
		fmt.Fprintf(os.Stderr, "acceptance sweep: deleting %s, left behind %s ago\n", p.Name, time.Since(p.CreatedAt).Round(time.Minute))
	}
	return nil
}

func TestSweepAccOrg(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/orgs":
			_ = json.NewEncoder(w).Encode([]client.Organization{{ID: "org-1", DisplayName: "tf-acc"}})
		case r.URL.Path == "/api/v1/orgs/org-1/projects":
			_ = json.NewEncoder(w).Encode([]client.Project{
				{ID: "old", Name: "tfaold-1", Status: "active", CreatedAt: time.Now().Add(-3 * time.Hour)},
				{ID: "young", Name: "tfayoung-1", Status: "active", CreatedAt: time.Now().Add(-10 * time.Minute)},
				{ID: "going", Name: "tfagoing-1", Status: "deleting", CreatedAt: time.Now().Add(-3 * time.Hour)},
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"))
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(client.Project{Status: "deleting"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")

	t.Setenv("FPCLOUD_ACC_SWEEP_ORG", "")
	if err := sweepAccOrg(context.Background()); err != nil || len(deleted) != 0 {
		t.Fatalf("unset org must sweep nothing: err=%v deleted=%v", err, deleted)
	}

	t.Setenv("FPCLOUD_ACC_SWEEP_ORG", "someone-elses-org")
	if err := sweepAccOrg(context.Background()); err == nil || len(deleted) != 0 {
		t.Fatalf("an org the key cannot see must refuse: err=%v deleted=%v", err, deleted)
	}

	t.Setenv("FPCLOUD_ACC_SWEEP_ORG", "tf-acc")
	if err := sweepAccOrg(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "old" {
		t.Fatalf("only the old, live project is reclaimed; deleted %v", deleted)
	}
}

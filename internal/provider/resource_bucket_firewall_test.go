package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

// A website's firewall is attached in the create of its bucket, before the
// website is enabled and anything is served, and taken off before that
// website goes, each waiting for the edge, and a change
// to another setting keeps it: only a single read of the bucket is sure to
// carry the firewall, so a write's own answer is never what reaches state.
func TestBucket_WebsiteFirewallFollowsTheWebsite(t *testing.T) {
	terraformBinary(t)

	var mu sync.Mutex
	var calls []string
	bucket := client.Bucket{ID: "bkt-1", ProjectID: "proj-1", Name: "site", Status: "ready",
		Endpoint: "https://s3.example.com", Region: "eu", GlobalAlias: "site-proj-org"}
	exists, attached, pending, pendingSeen := false, false, 0, 0
	// read is the bucket as a single read reports it, the edge one read
	// further along.
	read := func() client.Bucket {
		out := bucket
		status := "applied"
		if pending > 0 {
			pending--
			pendingSeen++
			status = "pending"
		}
		switch {
		case attached && status == "applied":
			out.Firewall = &client.FirewallRef{ID: "fw-1", Name: "office", Revision: 4, AppliedRevision: 4, Status: status}
		case attached:
			out.Firewall = &client.FirewallRef{ID: "fw-1", Name: "office", Revision: 4, Status: status}
		case status == "pending":
			out.Firewall = &client.FirewallRef{Status: status}
		}
		return out
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodGet {
			calls = append(calls, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/proj-1/buckets":
			var req client.CreateBucketRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			exists = true
			// A firewall named on the create is the website's before the
			// bucket serves anything.
			if req.Firewall != "" {
				attached, pending = true, 2
				calls[len(calls)-1] += " firewall=" + req.Firewall
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(bucket)
		case !exists:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/buckets/bkt-1":
			_ = json.NewEncoder(w).Encode(read())
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/buckets/bkt-1/website":
			var req client.SetBucketWebsiteRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if !req.Enabled && attached {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "the website still has a firewall"})
				return
			}
			bucket.WebsiteEnabled, bucket.PublicRead = req.Enabled, req.Enabled
			bucket.WebsiteIndexDocument, bucket.WebsiteErrorDocument = "", ""
			if req.Enabled {
				bucket.WebsiteIndexDocument, bucket.WebsiteErrorDocument = "index.html", req.ErrorDocument
			}
			// Answered without the firewall, as a write may.
			_ = json.NewEncoder(w).Encode(bucket)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/buckets/bkt-1/public-read":
			bucket.PublicRead = false
			_ = json.NewEncoder(w).Encode(bucket)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/buckets/bkt-1/website/firewall":
			if !bucket.WebsiteEnabled {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "the bucket serves no website"})
				return
			}
			attached, pending = true, 2
			_ = json.NewEncoder(w).Encode(client.FirewallRef{ID: "fw-1", Name: "office", Revision: 4, Status: "pending"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/buckets/bkt-1/website/firewall":
			attached, pending = false, 2
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/buckets/bkt-1":
			exists = false
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")
	provider.SetFirewallPoll(t, time.Millisecond)

	config := func(settings string) string {
		return fmt.Sprintf(`
resource "fpcloud_bucket" "site" {
  project = "proj-1"
  name    = "site"
  %s
}`, settings)
	}
	// step asserts the writes an apply made, in order, and that it waited
	// through every pending read before returning.
	step := func(wantPending int, want ...string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if fmt.Sprint(calls) != fmt.Sprint(want) {
				return fmt.Errorf("the apply made %v, want %v", calls, want)
			}
			if pendingSeen != wantPending || pending != 0 {
				return fmt.Errorf("the apply waited through %d pending reads with %d left, want %d and none", pendingSeen, pending, wantPending)
			}
			calls, pendingSeen = nil, 0
			return nil
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(`website_enabled = true
  firewall        = "fw-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_bucket.site", "firewall", "fw-1"),
					step(2, "POST /api/v1/projects/proj-1/buckets firewall=fw-1", "PUT /api/v1/buckets/bkt-1/website"),
				),
			},
			{
				Config: config(`website_enabled        = true
  website_error_document = "404.html"
  firewall               = "fw-1"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_bucket.site", "firewall", "fw-1"),
					step(0, "PUT /api/v1/buckets/bkt-1/website"),
				),
			},
			{
				Config: config(`website_enabled = false
  public_read     = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fpcloud_bucket.site", "firewall"),
					step(2, "DELETE /api/v1/buckets/bkt-1/website/firewall", "PUT /api/v1/buckets/bkt-1/public-read", "PUT /api/v1/buckets/bkt-1/website"),
				),
			},
		},
	})
}

// A bucket names its project and its website's firewall by name or id, and
// holds each reference as written: the API answers with ids, and writing those
// back would make an apply by name inconsistent with its plan, while a wait
// keyed on the name would never see the edge enforce it. A rewrite onto the
// ids is an update that writes nothing.
func TestBucket_ProjectAndFirewallByName(t *testing.T) {
	terraformBinary(t)
	provider.SetFirewallTimeout(t, 3*time.Second)

	var mu sync.Mutex
	var writes []string
	bucket := client.Bucket{ID: "bkt-1", ProjectID: "proj-1", Name: "site", Status: "ready",
		Endpoint: "https://s3.example.com", Region: "eu", GlobalAlias: "site-proj-org"}
	office := client.FirewallRef{ID: "fw-1", Name: "office", Revision: 1, AppliedRevision: 1, Status: "applied"}
	exists, attached := false, false
	read := func() client.Bucket {
		out := bucket
		if attached {
			ref := office
			out.Firewall = &ref
		}
		return out
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodGet {
			writes = append(writes, r.Method+" "+r.URL.Path)
		}
		// The API resolves a project by name or id at the route boundary.
		for _, ref := range []string{"demo", "proj-1"} {
			if p := "/api/v1/projects/" + ref; r.URL.Path == p || strings.HasPrefix(r.URL.Path, p+"/") {
				r.URL.Path = "/api/v1/projects/proj-1" + strings.TrimPrefix(r.URL.Path, p)
			}
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/proj-1":
			_ = json.NewEncoder(w).Encode(client.Project{ID: "proj-1", Name: "demo"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/proj-1/firewalls":
			_ = json.NewEncoder(w).Encode([]client.Firewall{{ID: "fw-1", ProjectID: "proj-1", Name: "office", Revision: 1}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/proj-1/buckets":
			var req client.CreateBucketRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Firewall != "" && req.Firewall != office.ID && req.Firewall != office.Name {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			exists, attached = true, req.Firewall != ""
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(read())
		case !exists:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/buckets/bkt-1":
			_ = json.NewEncoder(w).Encode(read())
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/buckets/bkt-1/website":
			bucket.WebsiteEnabled, bucket.PublicRead, bucket.WebsiteIndexDocument = true, true, "index.html"
			_ = json.NewEncoder(w).Encode(bucket)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/buckets/bkt-1/website/firewall":
			attached = true
			_ = json.NewEncoder(w).Encode(office)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/buckets/bkt-1/website/firewall":
			attached = false
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/buckets/bkt-1":
			exists, attached = false, false
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")
	provider.SetFirewallPoll(t, time.Millisecond)

	config := func(project, firewall string) string {
		return fmt.Sprintf(`
resource "fpcloud_bucket" "site" {
  project         = %q
  name            = "site"
  website_enabled = true
  firewall        = %q
}`, project, firewall)
	}
	wrote := func(want ...string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if fmt.Sprint(writes) != fmt.Sprint(want) {
				return fmt.Errorf("the apply made %v, want %v", writes, want)
			}
			writes = nil
			return nil
		}
	}
	name := "fpcloud_bucket.site"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("demo", "office"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "demo"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
					resource.TestCheckResourceAttr(name, "firewall", "office"),
					resource.TestCheckResourceAttr(name, "firewall_id", "fw-1"),
					wrote("POST /api/v1/projects/demo/buckets", "PUT /api/v1/buckets/bkt-1/website"),
				),
			},
			{
				Config: config("proj-1", "fw-1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "project", "proj-1"),
					resource.TestCheckResourceAttr(name, "project_id", "proj-1"),
					resource.TestCheckResourceAttr(name, "firewall", "fw-1"),
					resource.TestCheckResourceAttr(name, "firewall_id", "fw-1"),
					wrote(),
				),
			},
			{
				// Back to the names: still nothing written.
				Config: config("demo", "office"),
				Check:  wrote(),
			},
		},
	})
}

package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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

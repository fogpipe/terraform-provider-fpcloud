package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccPreCheck asserts the environment needed to talk to a live fpcloud API is
// present. It runs only under `TF_ACC=1` (resource.Test skips otherwise), so a
// plain `go test ./...` never reaches it. A missing FPCLOUD_API_KEY under TF_ACC
// is a misconfiguration and fails loudly rather than silently skipping.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Fatal("FPCLOUD_API_KEY must be set for TF_ACC acceptance tests")
	}
}

// accName builds a unique DNS-1123 name short enough to survive being combined
// with the others.
//
// An app's hostname is a single label built from the organization, the project
// and the app name (ADR-032, ADR-058), so those three share 63 characters.
// acctest.RandomWithPrefix spends 20 of them on its own suffix, which put every
// app-creating test over the limit and failed them all — correctly, and with a
// message naming exactly this. Eight random characters are unique enough for a
// throwaway org and leave room for a tag saying which test left the resource
// behind.
func accName(tag string) string {
	return "tfa" + tag + "-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
}

// testAccAppScaffold renders a real project + always-on public app that
// dependent-resource acceptance tests (app_config, webhook, domain) can attach
// to. Referencing real IDs — rather than literal "test-app"/"test-project"
// strings — is what keeps those creates from being rejected as forbidden. The
// app is always-on + ingress=all so custom domains are accepted (ADR-030).
// Reference fpcloud_project.scaffold.id / fpcloud_app.scaffold.id from the
// caller's config.
func testAccAppScaffold(projectName, appName string) string {
	return fmt.Sprintf(`
resource "fpcloud_project" "scaffold" {
  name = %[1]q
}

resource "fpcloud_app" "scaffold" {
  project_id = fpcloud_project.scaffold.id
  name       = %[2]q
  image      = "nginx:latest"
  ingress    = "all"
}
`, projectName, appName)
}

// testAccClient builds an API client from the same environment variables the
// provider reads, for use in CheckDestroy assertions.
func testAccClient() *client.Client {
	apiURL := os.Getenv("FPCLOUD_API_URL")
	if apiURL == "" {
		apiURL = "https://api.cloud.fogpipe.com"
	}
	return client.New(apiURL, os.Getenv("FPCLOUD_API_KEY"))
}

// isNotFoundErr reports whether err is a 404 from the API.
func isNotFoundErr(err error) bool {
	apiErr, ok := err.(*client.APIError)
	return ok && apiErr.StatusCode == 404
}

// isGoneErr reports whether a resource is effectively gone. A test tears down the
// resource and its parent project together; once the project is deleted its
// scoped resources are no longer authorizable, so the API answers 403 rather than
// 404. Treat both as "destroyed" in CheckDestroy.
func isGoneErr(err error) bool {
	apiErr, ok := err.(*client.APIError)
	return ok && (apiErr.StatusCode == 404 || apiErr.StatusCode == 403)
}

// testAccCheckBucketDestroy verifies every fpcloud_bucket in state is gone from
// the live API after the test tears down.
func testAccCheckBucketDestroy(s *terraform.State) error {
	c := testAccClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "fpcloud_bucket" {
			continue
		}
		_, err := c.GetBucket(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("bucket %s still exists", rs.Primary.ID)
		}
		if !isGoneErr(err) {
			return fmt.Errorf("unexpected error checking bucket %s destroy: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

// testAccCheckAppDestroy verifies every fpcloud_app in state is gone from the
// live API after the test tears down.
func testAccCheckAppDestroy(s *terraform.State) error {
	c := testAccClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "fpcloud_app" {
			continue
		}
		_, err := c.GetApp(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("app %s still exists", rs.Primary.ID)
		}
		if !isGoneErr(err) {
			return fmt.Errorf("unexpected error checking app %s destroy: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

// testAccOrgID resolves the organization the acceptance key operates in, for
// the org-scoped resources (secrets, billing) whose configs need a literal org
// id. When FPCLOUD_ACC_SWEEP_ORG names the throwaway org — as CI does — that
// name wins; otherwise a key that sees exactly one org uses it. Anything else
// fails rather than guessing: these tests write real org state.
func testAccOrgID(t *testing.T) string {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test, set TF_ACC=1")
	}
	orgs, err := testAccClient().ListOrgs(context.Background())
	if err != nil {
		t.Fatalf("listing orgs: %v", err)
	}
	if name := os.Getenv("FPCLOUD_ACC_SWEEP_ORG"); name != "" {
		for _, o := range orgs {
			if o.Name == name {
				return o.ID
			}
		}
		t.Fatalf("FPCLOUD_ACC_SWEEP_ORG=%q names no org this key can see", name)
	}
	if len(orgs) == 1 {
		return orgs[0].ID
	}
	t.Fatalf("key sees %d orgs; set FPCLOUD_ACC_SWEEP_ORG to name the throwaway one", len(orgs))
	return ""
}

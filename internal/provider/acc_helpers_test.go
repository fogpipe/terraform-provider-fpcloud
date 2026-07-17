package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
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
		if !isNotFoundErr(err) {
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
		if !isNotFoundErr(err) {
			return fmt.Errorf("unexpected error checking app %s destroy: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

package provider_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAppCreate_RefusesPersistentVolume(t *testing.T) {
	terraformBinary(t)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("FPCLOUD_API_URL", srv.URL)
	t.Setenv("FPCLOUD_API_KEY", "fp-test")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "fpcloud_app" "web" {
  project_id = "proj-1"
  name       = "web"
  image      = "ghcr.io/acme/web:v1"
  storage    = "10Gi"
}`,
				ExpectError: regexp.MustCompile("A new app cannot have a persistent volume"),
			},
		},
	})
	if n := calls.Load(); n != 0 {
		t.Fatalf("the refusal reached the API %d times; it must be decided at plan", n)
	}
}

package provider_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

// An attribute that takes another resource "by name or id" records the
// reference as written, and a computed id beside it holds the id it resolved
// to, which is what a plan compares: `<attribute>_id`, or `organization_id`
// beside `org`, as on fpcloud_project. Without the pair the attribute either
// holds the API's id, and every apply of a name is inconsistent with its plan,
// or it is compared as a string, and a rewrite from a name to the id replaces
// the resource. RequiresReplace on the reference is the second mistake
// whatever the partner says.
//
// Asserted over the schema every resource serves, keyed on the wording the
// descriptions use for such an attribute. It reaches only that wording: a
// reference described another way is not checked, which is why the expected
// set below is listed, so that rewording one away from the phrase fails here
// instead of dropping it from the check. The other half is the name: an input
// called `*_id` says it takes an id, so one that takes a name as well, or one
// of the ids a route resolves by name, is refused whatever its description.
func TestReferenceByNameOrIDHasAnIDPartner(t *testing.T) {
	ctx := context.Background()
	p := provider.New("test")()
	resources := p.(interface {
		Resources(context.Context) []func() fwresource.Resource
	}).Resources(ctx)

	found := map[string]bool{}
	for _, newResource := range resources {
		r := newResource()
		var meta fwresource.MetadataResponse
		r.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "fpcloud"}, &meta)
		var resp fwresource.SchemaResponse
		r.Schema(ctx, fwresource.SchemaRequest{}, &resp)

		for name, a := range resp.Schema.Attributes {
			at := meta.TypeName + "." + name
			description := strings.ToLower(a.GetDescription())
			input := a.IsRequired() || a.IsOptional()
			if input && strings.HasSuffix(name, "_id") {
				for _, spelling := range []string{"name or id", "id or name"} {
					if strings.Contains(description, spelling) {
						t.Errorf("%s is named for an id and takes a name; call it %s and give it a computed id", at, strings.TrimSuffix(name, "_id"))
					}
				}
				for _, scope := range []string{"project_id", "org_id", "organization_id"} {
					if name == scope {
						t.Errorf("%s is an input, but the API resolves a %s by name or id; it is a reference beside a computed id", at, strings.TrimSuffix(scope, "_id"))
					}
				}
			}
			if !strings.Contains(description, "by name or id") {
				continue
			}
			found[at] = true
			if s, ok := a.(schema.StringAttribute); ok {
				for _, m := range s.PlanModifiers {
					if strings.Contains(m.Description(ctx), "destroy and recreate") {
						t.Errorf("%s replaces the resource when its spelling changes; %s_id decides that", at, name)
					}
				}
			} else {
				t.Errorf("%s is %T; a reference is a string", at, a)
			}
			partnerName := name + "_id"
			if name == "org" {
				partnerName = "organization_id"
			}
			partner, ok := resp.Schema.Attributes[partnerName]
			switch {
			case !ok:
				t.Errorf("%s takes a reference by name or id and has no %s beside it", at, partnerName)
			case !partner.IsComputed() || partner.IsOptional() || partner.IsRequired():
				t.Errorf("%s.%s: want computed only, the id %s resolved to", meta.TypeName, partnerName, name)
			}
			if _, ok := r.(fwresource.ResourceWithModifyPlan); !ok {
				t.Errorf("%s: nothing resolves a changed spelling, so nothing replaces the resource when it names another", at)
			}
		}
	}

	want := []string{
		"fpcloud_app.firewall", "fpcloud_app.project",
		"fpcloud_billing_budget.org",
		"fpcloud_bucket.firewall", "fpcloud_bucket.project",
		"fpcloud_database.project",
		"fpcloud_firewall.project",
		"fpcloud_iam_binding.org", "fpcloud_iam_binding.project",
		"fpcloud_job.app", "fpcloud_job.project",
		"fpcloud_oidc_federation.project",
		"fpcloud_project.org",
		"fpcloud_registry_retention_policy.project",
		"fpcloud_registry_visibility.project",
		"fpcloud_runner.project",
		"fpcloud_secret.org", "fpcloud_secret.project",
		"fpcloud_service_account.org", "fpcloud_service_account.project",
	}
	for _, at := range want {
		if !found[at] {
			t.Errorf("%s no longer says it takes a reference by name or id, so nothing here checks it", at)
		}
	}
	if len(found) < len(want) {
		got := make([]string, 0, len(found))
		for at := range found {
			got = append(got, at)
		}
		sort.Strings(got)
		t.Logf("checked %v", got)
	}
}

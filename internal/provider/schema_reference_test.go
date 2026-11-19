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
// reference as written, and a computed `<attribute>_id` beside it holds the id
// it resolved to, which is what a plan compares. Without the pair the
// attribute either holds the API's id, and every apply of a name is
// inconsistent with its plan, or it is compared as a string, and a rewrite
// from a name to the id replaces the resource. RequiresReplace on the
// reference is the second mistake whatever the partner says.
//
// Asserted over the schema every resource serves, keyed on the wording the
// descriptions use for such an attribute. It reaches only that wording: a
// reference described another way is not checked, which is why the expected
// set below is listed, so that rewording one away from the phrase fails here
// instead of dropping it from the check.
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
			if !strings.Contains(strings.ToLower(a.GetDescription()), "by name or id") {
				continue
			}
			at := meta.TypeName + "." + name
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
			partner, ok := resp.Schema.Attributes[name+"_id"]
			switch {
			case !ok:
				t.Errorf("%s takes a reference by name or id and has no %s_id beside it", at, name)
			case !partner.IsComputed() || partner.IsOptional() || partner.IsRequired():
				t.Errorf("%s_id: want computed only, the id %s resolved to", at, name)
			}
		}
	}

	want := []string{"fpcloud_app.firewall", "fpcloud_bucket.firewall", "fpcloud_bucket.project", "fpcloud_firewall.project"}
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

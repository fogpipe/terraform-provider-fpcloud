package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/fogpipe/cloud-cli/pkg/client"
)

func tfStrings(t *testing.T, vals ...string) types.List {
	t.Helper()
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

// The API echoes command, args and release_command; a config that leaves one
// out has null, and the API answers with nothing — the two must read as equal.
func TestStringListFromAPI(t *testing.T) {
	if got := stringListFromAPI(types.ListNull(types.StringType), nil); !got.IsNull() {
		t.Fatalf("null + empty should stay null, got %v", got)
	}
	if got := stringListFromAPI(types.ListUnknown(types.StringType), nil); !got.IsNull() {
		t.Fatalf("unknown + empty should read as null, got %v", got)
	}
	empty := tfStrings(t)
	if got := stringListFromAPI(empty, nil); !got.Equal(empty) {
		t.Fatalf("an explicit empty list should survive an empty answer, got %v", got)
	}
	if got := stringListFromAPI(types.ListNull(types.StringType), []string{"serve", "/etc/zot/config.json"}); !got.Equal(tfStrings(t, "serve", "/etc/zot/config.json")) {
		t.Fatalf("values should be read back, got %v", got)
	}
	if got := stringListFromAPI(tfStrings(t, "old"), []string{"new"}); !got.Equal(tfStrings(t, "new")) {
		t.Fatalf("the API's value wins over the prior, got %v", got)
	}
}

func TestSetVolumeMountsOnModel(t *testing.T) {
	var diags diag.Diagnostics
	model := &AppResourceModel{}

	setVolumeMountsOnModel(model, nil, &diags)
	if !model.VolumeMounts.IsNull() {
		t.Fatalf("no mounts should read as null, got %v", model.VolumeMounts)
	}

	setVolumeMountsOnModel(model, []client.VolumeMount{
		{Source: "configmap", Name: "registry-config", MountPath: "/etc/zot"},
		{Source: "emptydir", MountPath: "/tmp"},
	}, &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	var got []VolumeMountModel
	diags.Append(model.VolumeMounts.ElementsAs(context.Background(), &got, false)...)
	if len(got) != 2 {
		t.Fatalf("want 2 mounts, got %d", len(got))
	}
	if got[0].Name.ValueString() != "registry-config" || !got[0].SubPath.IsNull() {
		t.Fatalf("an omitted sub_path must read as null, got %+v", got[0])
	}
	if !got[1].Name.IsNull() {
		t.Fatalf("an emptydir's absent name must read as null, got %+v", got[1])
	}
}

func TestSetSecurityContextOnModel(t *testing.T) {
	var diags diag.Diagnostics
	model := &AppResourceModel{}
	uid := int64(1000)

	setSecurityContextOnModel(model, nil, &diags)
	if !model.SecurityContext.IsNull() {
		t.Fatalf("no context should read as null, got %v", model.SecurityContext)
	}

	// The booleans are omitted on the wire when false and default to false in
	// the schema, so an absent one reads as a concrete false, never null.
	setSecurityContextOnModel(model, &client.SecurityContext{RunAsUser: &uid}, &diags)
	var got SecurityContextModel
	diags.Append(model.SecurityContext.As(context.Background(), &got, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if got.RunAsUser.ValueInt64() != 1000 || !got.RunAsGroup.IsNull() || got.RunAsNonRoot.IsNull() || got.RunAsNonRoot.ValueBool() || got.ReadOnlyRootFilesystem.ValueBool() {
		t.Fatalf("unexpected read-back: %+v", got)
	}

	// A change made outside Terraform is visible.
	setSecurityContextOnModel(model, &client.SecurityContext{RunAsUser: &uid, RunAsNonRoot: true, ReadOnlyRootFilesystem: true}, &diags)
	diags.Append(model.SecurityContext.As(context.Background(), &got, basetypes.ObjectAsOptions{})...)
	if !got.RunAsNonRoot.ValueBool() || !got.ReadOnlyRootFilesystem.ValueBool() {
		t.Fatalf("drift must be read back, got %+v", got)
	}
}

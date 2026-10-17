package provider_test

import (
	"context"
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The API refuses an empty value, so plan has to: Terraform counts "" as set,
// and the refusal otherwise lands mid-apply (fogpipe/cloud-workspace#1278).
// The non-empty twin is what shows the refusal is about the value.
func TestSecretEmptyValueIsRefusedAtPlan(t *testing.T) {
	ctx := context.Background()
	res := provider.NewSecretResource().(fwresource.ResourceWithValidateConfig)
	var sch fwresource.SchemaResponse
	res.Schema(ctx, fwresource.SchemaRequest{}, &sch)
	typ := sch.Schema.Type().TerraformType(ctx).(tftypes.Object)

	validate := func(value tftypes.Value) []string {
		attrs := map[string]tftypes.Value{}
		for name, at := range typ.AttributeTypes {
			attrs[name] = tftypes.NewValue(at, nil)
		}
		attrs["project_id"] = tftypes.NewValue(tftypes.String, "p")
		attrs["name"] = tftypes.NewValue(tftypes.String, "stripe-key")
		attrs["value"] = value
		var resp fwresource.ValidateConfigResponse
		res.ValidateConfig(ctx, fwresource.ValidateConfigRequest{
			Config: tfsdk.Config{Schema: sch.Schema, Raw: tftypes.NewValue(typ, attrs)},
		}, &resp)
		out := []string{}
		for _, d := range resp.Diagnostics.Errors() {
			out = append(out, d.Summary())
		}
		return out
	}

	if errs := validate(tftypes.NewValue(tftypes.String, "")); len(errs) != 1 || errs[0] != "A secret holds a value" {
		t.Errorf("empty value: got %v, want the value refused", errs)
	}
	if errs := validate(tftypes.NewValue(tftypes.String, "sk_test_123")); len(errs) != 0 {
		t.Errorf("non-empty value: got %v, want no error", errs)
	}
	if errs := validate(tftypes.NewValue(tftypes.String, tftypes.UnknownValue)); len(errs) != 0 {
		t.Errorf("unknown value: got %v, want it left to apply", errs)
	}
}

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
)

// probeObject builds one liveness/readiness/startup block, defaulting every
// attribute the caller does not name to null — which is what "inherit the app's
// health check" looks like in config.
func probeObject(t *testing.T, attrs map[string]attr.Value) types.Object {
	t.Helper()
	values := map[string]attr.Value{
		"path":                  types.StringNull(),
		"initial_delay_seconds": types.Int64Null(),
		"period_seconds":        types.Int64Null(),
		"timeout_seconds":       types.Int64Null(),
		"failure_threshold":     types.Int64Null(),
		"success_threshold":     types.Int64Null(),
	}
	for k, v := range attrs {
		values[k] = v
	}
	obj, d := types.ObjectValue(probeSpecAttrTypes(), values)
	if d.HasError() {
		t.Fatalf("build probe object: %v", d)
	}
	return obj
}

func probesObject(t *testing.T, liveness, readiness, startup attr.Value) types.Object {
	t.Helper()
	obj, d := types.ObjectValue(probesAttrTypes(), map[string]attr.Value{
		"liveness":  liveness,
		"readiness": readiness,
		"startup":   startup,
	})
	if d.HasError() {
		t.Fatalf("build probes object: %v", d)
	}
	return obj
}

func nullProbe() types.Object { return types.ObjectNull(probeSpecAttrTypes()) }

func TestProbesFromModel(t *testing.T) {
	ctx := context.Background()

	t.Run("null block leaves every probe on the health check", func(t *testing.T) {
		var diags diag.Diagnostics
		if got := probesFromModel(ctx, types.ObjectNull(probesAttrTypes()), &diags); got != nil {
			t.Fatalf("want nil, got %#v", got)
		}
	})

	// An unknown block is not a request to change anything. Sending it would
	// serialize to no overrides and silently clear what the app already has.
	t.Run("unknown block is not sent", func(t *testing.T) {
		var diags diag.Diagnostics
		if got := probesFromModel(ctx, types.ObjectUnknown(probesAttrTypes()), &diags); got != nil {
			t.Fatalf("want nil, got %#v", got)
		}
	})

	// A block naming all three probes as null carries no override, so it must not
	// travel as an empty object the server would store as "overridden with nothing".
	t.Run("all three null collapses to nil", func(t *testing.T) {
		var diags diag.Diagnostics
		got := probesFromModel(ctx, probesObject(t, nullProbe(), nullProbe(), nullProbe()), &diags)
		if got != nil {
			t.Fatalf("want nil, got %#v", got)
		}
	})

	t.Run("overriding one probe leaves the others nil", func(t *testing.T) {
		var diags diag.Diagnostics
		got := probesFromModel(ctx, probesObject(t,
			probeObject(t, map[string]attr.Value{"path": types.StringValue("/livez")}),
			nullProbe(),
			nullProbe(),
		), &diags)
		if got == nil || got.Liveness == nil {
			t.Fatalf("want a liveness override, got %#v", got)
		}
		if got.Liveness.Path != "/livez" {
			t.Errorf("want /livez, got %q", got.Liveness.Path)
		}
		// Unset timings must serialize as zero so omitempty drops them and the
		// server falls back to health_check_*, rather than pinning them here.
		if got.Liveness.PeriodSeconds != 0 || got.Liveness.TimeoutSeconds != 0 {
			t.Errorf("unset timings must stay zero, got %#v", got.Liveness)
		}
		if got.Readiness != nil || got.Startup != nil {
			t.Errorf("want the other probes nil, got %#v", got)
		}
	})

	t.Run("converts every field", func(t *testing.T) {
		var diags diag.Diagnostics
		got := probesFromModel(ctx, probesObject(t, nullProbe(), probeObject(t, map[string]attr.Value{
			"path":                  types.StringValue("/ready"),
			"initial_delay_seconds": types.Int64Value(5),
			"period_seconds":        types.Int64Value(10),
			"timeout_seconds":       types.Int64Value(2),
			"failure_threshold":     types.Int64Value(3),
			"success_threshold":     types.Int64Value(2),
		}), nullProbe()), &diags)
		want := client.ProbeSpec{
			Path:                "/ready",
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      2,
			FailureThreshold:    3,
			SuccessThreshold:    2,
		}
		if got == nil || got.Readiness == nil || *got.Readiness != want {
			t.Fatalf("want %+v, got %#v", want, got)
		}
	})
}

func TestSetProbesOnModel(t *testing.T) {
	t.Run("no overrides yields a null block", func(t *testing.T) {
		var model AppResourceModel
		var diags diag.Diagnostics
		setProbesOnModel(&model, nil, &diags)
		if !model.Probes.IsNull() {
			t.Fatalf("want null object, got %v", model.Probes)
		}
		if model.Probes.IsUnknown() {
			t.Fatal("object must not be left unknown")
		}
	})

	// The API omits a field it does not have, which decodes to Go's zero value.
	// Writing that back as 0 where the config said nothing would be an
	// inconsistent-result error after apply — and 0 is not what unset means.
	t.Run("fields the API omits come back null, not zero", func(t *testing.T) {
		var model AppResourceModel
		var diags diag.Diagnostics
		setProbesOnModel(&model, &client.ProbeOverrides{
			Liveness: &client.ProbeSpec{Path: "/livez"},
		}, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		liveness := model.Probes.Attributes()["liveness"].(types.Object)
		for _, key := range []string{
			"initial_delay_seconds", "period_seconds", "timeout_seconds",
			"failure_threshold", "success_threshold",
		} {
			if !liveness.Attributes()[key].IsNull() {
				t.Errorf("%s: want null, got %v", key, liveness.Attributes()[key])
			}
		}
		if readiness := model.Probes.Attributes()["readiness"]; !readiness.IsNull() {
			t.Errorf("want a null readiness block, got %v", readiness)
		}
	})

	t.Run("round-trips through the model", func(t *testing.T) {
		var model AppResourceModel
		var diags diag.Diagnostics
		in := &client.ProbeOverrides{
			Liveness:  &client.ProbeSpec{Path: "/livez"},
			Readiness: &client.ProbeSpec{Path: "/ready", FailureThreshold: 2},
			Startup:   &client.ProbeSpec{FailureThreshold: 30},
		}
		setProbesOnModel(&model, in, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		back := probesFromModel(context.Background(), model.Probes, &diags)
		if back == nil {
			t.Fatal("round-trip lost the overrides")
		}
		if *back.Liveness != *in.Liveness || *back.Readiness != *in.Readiness || *back.Startup != *in.Startup {
			t.Fatalf("round-trip mismatch: %#v", back)
		}
	})
}

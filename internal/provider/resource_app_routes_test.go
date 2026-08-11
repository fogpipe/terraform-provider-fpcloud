package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fogpipe/cloud-cli/pkg/client"
)

func routeList(t *testing.T, routes ...map[string]attr.Value) types.List {
	t.Helper()
	objType := types.ObjectType{AttrTypes: routeAttrTypes()}
	elems := make([]attr.Value, len(routes))
	for i, r := range routes {
		obj, d := types.ObjectValue(routeAttrTypes(), r)
		if d.HasError() {
			t.Fatalf("build object: %v", d)
		}
		elems[i] = obj
	}
	list, d := types.ListValue(objType, elems)
	if d.HasError() {
		t.Fatalf("build list: %v", d)
	}
	return list
}

func TestRoutesFromModel(t *testing.T) {
	ctx := context.Background()
	objType := types.ObjectType{AttrTypes: routeAttrTypes()}

	t.Run("null list means no carve-outs", func(t *testing.T) {
		var diags diag.Diagnostics
		got := routesFromModel(ctx, types.ListNull(objType), &diags)
		if got != nil {
			t.Fatalf("want nil, got %v", got)
		}
	})

	t.Run("unknown list is not sent", func(t *testing.T) {
		var diags diag.Diagnostics
		if got := routesFromModel(ctx, types.ListUnknown(objType), &diags); got != nil {
			t.Fatalf("want nil, got %v", got)
		}
	})

	// An explicitly empty list is how a practitioner clears the carve-outs; it
	// must survive as a non-nil empty slice so the PUT body says "none" rather
	// than being indistinguishable from "unset".
	t.Run("empty list clears", func(t *testing.T) {
		var diags diag.Diagnostics
		got := routesFromModel(ctx, routeList(t), &diags)
		if got == nil || len(got) != 0 {
			t.Fatalf("want empty non-nil slice, got %#v", got)
		}
	})

	t.Run("converts paths and visibility", func(t *testing.T) {
		var diags diag.Diagnostics
		got := routesFromModel(ctx, routeList(t,
			map[string]attr.Value{"path": types.StringValue("/internal/"), "visibility": types.StringValue("internal")},
			map[string]attr.Value{"path": types.StringValue("/api/"), "visibility": types.StringValue("public")},
		), &diags)
		want := []client.Route{
			{Path: "/internal/", Visibility: "internal"},
			{Path: "/api/", Visibility: "public"},
		}
		if len(got) != len(want) {
			t.Fatalf("want %d routes, got %d", len(want), len(got))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("route %d: want %+v, got %+v", i, want[i], got[i])
			}
		}
	})

	// visibility is Optional+Computed, so it arrives unknown or null when omitted.
	// Sending "" would fail the server's enum check, so it must default here.
	t.Run("omitted visibility defaults to internal", func(t *testing.T) {
		for name, v := range map[string]attr.Value{
			"unknown": types.StringUnknown(),
			"null":    types.StringNull(),
			"empty":   types.StringValue(""),
		} {
			t.Run(name, func(t *testing.T) {
				var diags diag.Diagnostics
				got := routesFromModel(ctx, routeList(t,
					map[string]attr.Value{"path": types.StringValue("/internal/"), "visibility": v},
				), &diags)
				if len(got) != 1 || got[0].Visibility != "internal" {
					t.Fatalf("want internal visibility, got %#v", got)
				}
			})
		}
	})
}

func TestSetRoutesOnModel(t *testing.T) {
	// A Computed list left unknown after apply is an "invalid result object"
	// error from the framework, so the empty case must be an explicit null.
	t.Run("no routes yields a null list", func(t *testing.T) {
		var model AppResourceModel
		var diags diag.Diagnostics
		setRoutesOnModel(&model, nil, &diags)
		if !model.Routes.IsNull() {
			t.Fatalf("want null list, got %v", model.Routes)
		}
		if model.Routes.IsUnknown() {
			t.Fatal("list must not be left unknown")
		}
	})

	t.Run("round-trips through the model", func(t *testing.T) {
		var model AppResourceModel
		var diags diag.Diagnostics
		setRoutesOnModel(&model, []client.Route{{Path: "/internal/", Visibility: "internal"}}, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		back := routesFromModel(context.Background(), model.Routes, &diags)
		if len(back) != 1 || back[0].Path != "/internal/" || back[0].Visibility != "internal" {
			t.Fatalf("round-trip mismatch: %#v", back)
		}
	})
}

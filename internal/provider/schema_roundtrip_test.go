package provider_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
)

// A nested attribute inside an Optional+Computed collection is never Required.
//
// An Optional+Computed collection is one the provider fills in from the API when
// the configuration does not write it. Terraform builds its proposed new state
// by descending INTO the element the provider read back, and a nested attribute
// the configuration does not supply takes its value from what the schema allows:
// Optional+Computed takes the prior value, Required has nowhere to take one from
// and becomes null. So the proposal differs from prior state at an attribute
// nobody touched, and it can never be reconciled — worse, the difference makes
// the framework mark every OTHER config-null computed attribute unknown, so the
// whole resource plans as an update forever and a CI job gating on an empty plan
// never goes green again.
//
// It reached production as `fpcloud_app.traffic`, whose `revision` and `percent`
// were Required: only a serverless app has traffic at all, so the permanent diff
// appeared the moment an app was switched to serverless and nowhere else
// (fogpipe/cloud-workspace#226). Two more carried it latently.
//
// Asserted over the schema every resource actually serves, because what has to
// be true is a property of the schema tree rather than of one file — a new
// nested attribute is written beside the others and looks entirely ordinary.
func TestNoRequiredAttributeInsideAnOptionalComputedCollection(t *testing.T) {
	p := provider.New("test")()
	resources := p.(interface {
		Resources(context.Context) []func() fwresource.Resource
	}).Resources(context.Background())

	if len(resources) == 0 {
		t.Fatal("provider serves no resources; the walk is asserting nothing")
	}

	checked := 0
	for _, newResource := range resources {
		r := newResource()
		var meta fwresource.MetadataResponse
		r.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "fpcloud"}, &meta)

		var resp fwresource.SchemaResponse
		r.Schema(context.Background(), fwresource.SchemaRequest{}, &resp)

		for name, attr := range resp.Schema.Attributes {
			checked++
			walkAttribute(t, meta.TypeName, name, reflect.ValueOf(attr), false)
		}
	}
	if checked == 0 {
		t.Fatal("no attributes walked")
	}
}

// walkAttribute reports a Required attribute reached from inside an
// Optional+Computed collection. insideOptionalComputed says whether some
// ancestor was one; once true it stays true, because the proposal is nulled at
// every depth below the collection the configuration omitted.
func walkAttribute(t *testing.T, resource, path string, v reflect.Value, insideOptionalComputed bool) {
	t.Helper()

	for v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}

	flag := func(name string) bool {
		f := v.FieldByName(name)
		return f.IsValid() && f.Kind() == reflect.Bool && f.Bool()
	}

	if insideOptionalComputed && flag("Required") {
		t.Errorf("%s: %s is Required inside an Optional+Computed collection; "+
			"make it Optional+Computed so the proposed new state can take its prior value",
			resource, path)
	}

	nested := v.FieldByName("NestedObject")
	if !nested.IsValid() {
		return
	}
	attrs := nested.FieldByName("Attributes")
	if !attrs.IsValid() || attrs.Kind() != reflect.Map {
		return
	}

	below := insideOptionalComputed || (flag("Optional") && flag("Computed"))
	iter := attrs.MapRange()
	for iter.Next() {
		walkAttribute(t, resource, fmt.Sprintf("%s.%s", path, iter.Key().String()), iter.Value(), below)
	}
}

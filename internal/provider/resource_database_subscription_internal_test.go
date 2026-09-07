package provider

import (
	"context"
	"testing"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func subscriptionSchema(t *testing.T) fwresource.SchemaResponse {
	t.Helper()
	var resp fwresource.SchemaResponse
	NewDatabaseSubscriptionResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp
}

// eq fails with what it got, so a wrong value names itself.
func eq[T comparable](t *testing.T, got, want T, what string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestDatabaseSubscriptionSchemaIsValid(t *testing.T) {
	resp := subscriptionSchema(t)
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("invalid schema: %v", diags)
	}
}

// The password is write-only on every surface, and a schema that forgot to mark
// it sensitive would put a tenant's source credential into plan output and into
// the state file's diff.
func TestDatabaseSubscriptionPasswordIsSensitive(t *testing.T) {
	resp := subscriptionSchema(t)
	attr, diags := resp.Schema.AttributeAtPath(context.Background(),
		path.Root("source").AtName("password"))
	if diags.HasError() {
		t.Fatalf("no source.password attribute: %v", diags)
	}
	if !attr.IsSensitive() {
		t.Error("the source password must never be printed in a plan")
	}
	if !attr.IsRequired() {
		t.Error("the source password is required: there is no other place to give it")
	}
}

// A number nobody could read is -1, never 0. A subscription whose lag could not
// be taken must not render as one that is fully caught up — the reading is
// absent, and 0 is the healthiest value available.
func TestMapSubscriptionToStateSpellsAnUnreadNumber(t *testing.T) {
	var state DatabaseSubscriptionResourceModel
	diags := mapSubscriptionToState(context.Background(), &client.DatabaseSubscription{
		DatabaseID: "db-1", Name: "migrate", Publication: "everything",
		Source:  client.DatabaseSubscriptionSource{Host: "db.example.com", Port: 5432, User: "postgres", DBName: "postgres", SSLMode: "require"},
		Applied: true,
		Health: client.DatabaseSubscriptionHealth{
			State:      client.SubscriptionUnknown,
			Unreadable: "the database could not be asked",
		},
	}, types.StringValue("kept"), &state)
	if diags.HasError() {
		t.Fatalf("map: %v", diags)
	}

	health := state.Health.Attributes()
	eq(t, health["apply_lag_bytes"].(types.Int64), types.Int64Value(-1), "apply_lag_bytes")
	eq(t, health["apply_errors"].(types.Int64), types.Int64Value(-1), "apply_errors")
	eq(t, health["sync_errors"].(types.Int64), types.Int64Value(-1), "sync_errors")
	eq(t, health["state"].(types.String), types.StringValue("unknown"), "state")
	eq(t, health["unreadable"].(types.String), types.StringValue("the database could not be asked"), "unreadable")

	// `applied` is beside health, never inside it: the operand says the
	// statement ran, and that stays true of a subscription nobody can reach.
	eq(t, state.Applied, types.BoolValue(true), "applied")

	// The password no read returns is the one the caller held.
	eq(t, state.Source.Attributes()["password"].(types.String), types.StringValue("kept"), "source.password")
}

func TestMapSubscriptionToStateCarriesTheNumbersItHas(t *testing.T) {
	lag, apply, sync := int64(4096), int64(2), int64(0)
	at := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	var state DatabaseSubscriptionResourceModel
	diags := mapSubscriptionToState(context.Background(), &client.DatabaseSubscription{
		DatabaseID: "db-1", Name: "migrate",
		Source: client.DatabaseSubscriptionSource{Host: "db.example.com", Port: 6432},
		Health: client.DatabaseSubscriptionHealth{
			State: client.SubscriptionReplicating, ApplyLagBytes: &lag,
			ApplyErrors: &apply, SyncErrors: &sync, LastMessageAt: &at,
		},
	}, types.StringNull(), &state)
	if diags.HasError() {
		t.Fatalf("map: %v", diags)
	}

	health := state.Health.Attributes()
	eq(t, health["apply_lag_bytes"].(types.Int64), types.Int64Value(4096), "apply_lag_bytes")
	eq(t, health["apply_errors"].(types.Int64), types.Int64Value(2), "apply_errors")
	// Zero errors is a reading somebody took, and stays zero.
	eq(t, health["sync_errors"].(types.Int64), types.Int64Value(0), "sync_errors")
	eq(t, health["last_message_at"].(types.String), types.StringValue("2026-09-08T10:00:00Z"), "last_message_at")
	eq(t, state.Source.Attributes()["port"].(types.Int32), types.Int32Value(6432), "source.port")
	eq(t, state.ID, types.StringValue("db-1/migrate"), "id")
}

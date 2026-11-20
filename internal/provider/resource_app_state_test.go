package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A state written with a fixed replica count becomes the same count as both
// bounds, so a config rewritten from `replicas = 3` to `min_scale = 3` and
// `max_scale = 3` plans no change.
func TestUpgradeAppState_ReplicasBecomeEqualBounds(t *testing.T) {
	for _, tc := range []struct {
		prior string
		want  float64
	}{
		{`{"id":"a","replicas":3,"min_scale":0,"max_scale":10,"image":"x","mode":"always-on","traffic":[]}`, 3},
		{`{"id":"a","replicas":0,"min_scale":0,"max_scale":10}`, 1},
		{`{"id":"a"}`, 1},
	} {
		got := upgradeAppState(t, 0, tc.prior)
		assert.Equal(t, tc.want, got["min_scale"], tc.prior)
		assert.Equal(t, tc.want, got["max_scale"], tc.prior)
		_, hasReplicas := got["replicas"]
		assert.False(t, hasReplicas, "replicas leaves the state")
		_, hasMode := got["mode"]
		assert.False(t, hasMode, "mode leaves the state")
		assert.Equal(t, "a", got["id"], "everything else is carried as it was")
	}
}

// Version 1's mounts become scratch directories: an emptydir is rewritten, a
// ConfigMap or Secret mount is dropped as the API dropped it, and none left
// reads as null. The version 0 upgrader applies the same step, since the
// framework hands each upgrader the state its own version wrote.
func TestUpgradeAppState_MountsBecomeScratchDirectories(t *testing.T) {
	prior := `{"id":"a","volume_mounts":[` +
		`{"source":"secret","name":"p-a-db-ca","mount_path":"/etc/cnpg-ca/ca.crt","sub_path":"ca.crt"},` +
		`{"source":"emptydir","name":null,"mount_path":"/data","sub_path":null}]}`
	for _, version := range []int64{0, 1} {
		got := upgradeAppState(t, version, prior)
		assert.Equal(t, []any{map[string]any{"mount_path": "/data", "empty_dir": map[string]any{}}}, got["volume_mounts"], "from version %d", version)
		assert.Equal(t, "a", got["id"])
	}

	got := upgradeAppState(t, 1, `{"id":"a","volume_mounts":[{"source":"configmap","name":"p-a-config","mount_path":"/etc/app","sub_path":null}]}`)
	assert.Nil(t, got["volume_mounts"], "no mount left reads as null, as the API's readback does")

	got = upgradeAppState(t, 1, `{"id":"a","volume_mounts":null}`)
	assert.Nil(t, got["volume_mounts"])
}

func TestUpgradeAppState_EveryAttributeTheSchemaLostIsDropped(t *testing.T) {
	prior := `{"id":"a","name":"grannsnack","image":"x","replicas":2,"mode":"always-on","traffic":[],` +
		`"database":"db","secret":{"API_KEY":"k"},"env":{"A":"1"},"health_check_path":"/healthz"}`
	for _, version := range []int64{0, 1, 2, 3} {
		got := upgradeAppState(t, version, prior)
		for _, gone := range []string{"database", "secret", "mode", "traffic", "replicas", "health_check_path"} {
			_, kept := got[gone]
			assert.False(t, kept, "%s leaves the state from version %d", gone, version)
		}
		assert.Equal(t, "grannsnack", got["name"])
	}
}

func upgradeAppState(t *testing.T, version int64, prior string) map[string]any {
	t.Helper()
	upgrader, ok := (&AppResource{}).UpgradeState(context.Background())[version]
	require.True(t, ok, "an upgrader from version %d", version)
	var resp resource.UpgradeStateResponse
	upgrader.StateUpgrader(context.Background(), resource.UpgradeStateRequest{
		RawState: &tfprotov6.RawState{JSON: []byte(prior)},
	}, &resp)
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	_, err := resp.DynamicValue.Unmarshal(schemaType(context.Background(), &AppResource{}))
	require.NoError(t, err, "the upgraded state decodes against the current schema")
	var got map[string]any
	require.NoError(t, json.Unmarshal(resp.DynamicValue.JSON, &got))
	return got
}

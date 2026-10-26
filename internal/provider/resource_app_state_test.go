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
		var resp resource.UpgradeStateResponse
		upgradeAppStateV0(context.Background(), resource.UpgradeStateRequest{
			RawState: &tfprotov6.RawState{JSON: []byte(tc.prior)},
		}, &resp)
		require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
		var got map[string]any
		require.NoError(t, json.Unmarshal(resp.DynamicValue.JSON, &got))
		assert.Equal(t, tc.want, got["min_scale"], tc.prior)
		assert.Equal(t, tc.want, got["max_scale"], tc.prior)
		_, hasReplicas := got["replicas"]
		assert.False(t, hasReplicas, "replicas leaves the state")
		_, hasMode := got["mode"]
		assert.False(t, hasMode, "mode leaves the state")
		assert.Equal(t, "a", got["id"], "everything else is carried as it was")
	}
}

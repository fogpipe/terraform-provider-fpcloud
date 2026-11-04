package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fogpipe/cloud-cli/pkg/client"
)

// A deploy the cluster refused is answered 200 with the app marked failed and
// the reason on the deployment record, whether or not the app has a release
// command. Reading the record only behind a release command made that an apply
// that succeeded while the previous version went on serving.
func TestAwaitDeployment(t *testing.T) {
	const refusal = "observe domains for web: list challenges in fp-acme: forbidden"
	cases := []struct {
		name    string
		app     client.App
		records []client.Deployment
		wantErr string
	}{
		{
			name:    "a failed record fails the apply with the API's message",
			app:     client.App{ID: "app-1", Status: "failed"},
			records: []client.Deployment{{ID: "dep-1", Status: "failed", Message: refusal}},
			wantErr: "the deploy failed: " + refusal,
		},
		{
			name:    "a failed record with no message still fails the apply",
			app:     client.App{ID: "app-1", Status: "failed"},
			records: []client.Deployment{{ID: "dep-1", Status: "failed"}},
			wantErr: "the deploy failed; `fpcloud app deployments app-1` has its record",
		},
		{
			name:    "an app marked failed with no record still fails the apply",
			app:     client.App{ID: "app-1", Status: "failed"},
			wantErr: "the deploy failed; `fpcloud app deployments app-1` has its record",
		},
		{
			name:    "an active record is an apply that succeeded",
			app:     client.App{ID: "app-1", Status: "deploying"},
			records: []client.Deployment{{ID: "dep-1", Status: "active"}},
		},
		{
			name: "only the newest record decides",
			app:  client.App{ID: "app-1", Status: "deploying"},
			records: []client.Deployment{
				{ID: "dep-2", Status: "active"},
				{ID: "dep-1", Status: "failed", Message: refusal},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/apps/app-1/deployments" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				records := tc.records
				if records == nil {
					records = []client.Deployment{}
				}
				_ = json.NewEncoder(w).Encode(records)
			}))
			defer srv.Close()

			err := awaitDeployment(context.Background(), client.New(srv.URL, "fp-test"), &tc.app)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("awaitDeployment: %v", err)
			case tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr):
				t.Fatalf("awaitDeployment = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

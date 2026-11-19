package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/fogpipe/cloud-cli/pkg/client"
)

// Only a create answered ALREADY_EXISTS names a resource to adopt: every other
// 409 the platform answers is a refusal whose message is the one to show.
func TestIsAlreadyExistsAdoptsOnlyAnExistingResource(t *testing.T) {
	for _, tc := range []struct {
		code string
		want bool
	}{
		{"ALREADY_EXISTS", true},
		{"NAME_HELD", false},
		{"PROJECT_TEARING_DOWN", false},
		{"FIREWALL_ATTACHED", false},
		{"", false},
	} {
		err := fmt.Errorf("create: %w", &client.APIError{StatusCode: http.StatusConflict, Code: tc.code})
		if got := isAlreadyExists(err); got != tc.want {
			t.Errorf("isAlreadyExists(409 %q) = %v, want %v", tc.code, got, tc.want)
		}
	}
	if isAlreadyExists(&client.APIError{StatusCode: http.StatusBadRequest, Code: "ALREADY_EXISTS"}) {
		t.Error("a code on any status but 409 is not a conflict")
	}
}

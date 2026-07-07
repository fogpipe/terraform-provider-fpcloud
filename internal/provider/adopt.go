package provider

import (
	"errors"
	"net/http"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
)

// isConflict reports whether err is an API 409 Conflict — the status the
// control plane returns when a resource of the same name already exists
// (org/project/app uniqueness, SQLSTATE 23505). Create handlers use it to
// distinguish "name already taken" from any other create failure so the
// opt-in adopt_existing path can take over management of the existing resource
// instead of hard-failing the apply.
func isConflict(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}

// isNotFound reports whether err is an API 404. Import handlers use it to fall
// back from an id lookup to a name lookup.
func isNotFound(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

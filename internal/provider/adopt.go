package provider

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
)

// errNotAccessible marks the case where a resource could not be resolved by
// name because it is not among the resources the current API token is allowed
// to see. The list endpoints are IAM-scoped to the caller, so a name that is
// absent from the list — despite create returning 409 Conflict (it exists) —
// means the token lacks access to adopt it. Adoption and import handlers use
// errors.Is to tell this apart from a transient list failure and emit a precise
// message.
var errNotAccessible = errors.New("not found among the resources accessible to the current API token")

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

// adoptErrorDetail renders the diagnostic detail when adopt_existing hits a 409
// (the named resource exists) but resolving it by name fails. It distinguishes
// "exists but the token can't access it" (errNotAccessible) — the common,
// actionable case — from a transient lookup failure, so the user fixes access
// rather than chasing a phantom "not found".
func adoptErrorDetail(kind, name string, err error) string {
	if errors.Is(err, errNotAccessible) {
		return fmt.Sprintf(
			"%s %q already exists (create returned 409 Conflict) but is not accessible with the current "+
				"API token, so it cannot be adopted. Grant the token's identity access (an IAM binding) to "+
				"the %s, or drop adopt_existing.",
			kind, name, kind,
		)
	}
	return fmt.Sprintf("%s %q already exists but looking it up for adoption failed: %s", kind, name, err.Error())
}

package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A reference attribute names another resource the way a person wrote it, and
// the API answers to a name and an id alike. So the attribute keeps what was
// written and a computed `<attribute>_id` beside it holds the id it resolved
// to, which is what a plan compares: comparing the strings would make a
// rewrite from `fpcloud_project.x.name` to `fpcloud_project.x.id` replace the
// resource, and writing the API's id back into the attribute would make every
// apply of a name inconsistent with its own plan. fpcloud_project's `org` and
// `organization_id` are the same pair.

// planProjectReference decides what a changed `project` on a project-scoped
// resource is, and returns the `project_id` the plan carries and whether the
// change replaces the resource. An unchanged reference costs no request. A
// changed one is resolved once: the same project spelled differently is an
// update in place that writes nothing, a different project replaces, and one
// that resolves to nothing fails the plan. An unknown reference replaces, since
// nothing can show it is the same project.
//
// State written before `project_id` existed has none, and its `project` then
// holds the id the API answered with, so that is the id compared against.
func planProjectReference(ctx context.Context, c *client.Client, config, stateRef, stateID types.String, diags *diag.Diagnostics) (types.String, bool) {
	frozen := stateID
	if frozen.IsNull() || frozen.IsUnknown() || frozen.ValueString() == "" {
		frozen = stateRef
	}
	if config.IsUnknown() {
		return types.StringUnknown(), true
	}
	if config.Equal(stateRef) {
		return frozen, false
	}
	project, err := c.GetProject(ctx, config.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root("project"), "Error resolving project",
			fmt.Sprintf("%q could not be resolved to a project, so the plan cannot tell another spelling of "+
				"the same project from a different one: %s", config.ValueString(), err))
		return frozen, false
	}
	if project.ID == frozen.ValueString() {
		return types.StringValue(project.ID), false
	}
	return types.StringUnknown(), true
}

// setProjectReference writes the project the API answered into `project_id`
// and leaves `project` as written. A resource read without one, an import, takes
// the id as its reference, so the attribute is never empty. An answer naming no
// project keeps what the model holds, so a computed attribute is never left
// unknown after an apply.
func setProjectReference(ref, id *types.String, answered string) {
	switch {
	case answered != "":
		*id = types.StringValue(answered)
	case id.IsNull() || id.IsUnknown():
		*id = *ref
	}
	if ref.IsNull() || ref.IsUnknown() || ref.ValueString() == "" {
		*ref = *id
	}
}

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A reference attribute names another resource the way a person wrote it, and
// the API answers to a name and an id alike. So the attribute keeps what was
// written and a computed id beside it holds the id it resolved to, which is
// what a plan compares: comparing the strings would make a rewrite from
// `fpcloud_project.x.name` to `fpcloud_project.x.id` replace the resource, and
// writing the API's id back into the attribute would make every apply of a
// name inconsistent with its own plan. fpcloud_project's `org` and
// `organization_id` are the same pair.

// referenceKind is one kind of resource a reference attribute names: the
// attribute holding the reference as written, the computed attribute holding
// the id it resolved to, and how a spelling is resolved.
type referenceKind struct {
	attr    string
	partner string
	// name and noun name the kind in a diagnostic, bare and with its article.
	name    string
	noun    string
	resolve func(ctx context.Context, c *client.Client, ref string) (string, error)
}

// projectReference is `project`, by name or id, beside its `project_id`.
var projectReference = referenceKind{
	attr:    "project",
	partner: "project_id",
	name:    "project",
	noun:    "a project",
	resolve: func(ctx context.Context, c *client.Client, ref string) (string, error) {
		project, err := c.GetProject(ctx, ref)
		if err != nil {
			return "", err
		}
		return project.ID, nil
	},
}

// orgReference is `org`, by uuid, opaque id or name, beside its
// `organization_id`, as on fpcloud_project.
var orgReference = referenceKind{
	attr:    "org",
	partner: "organization_id",
	name:    "organization",
	noun:    "an organization",
	resolve: func(ctx context.Context, c *client.Client, ref string) (string, error) {
		org, err := c.GetOrg(ctx, ref)
		if err != nil {
			return "", err
		}
		return org.ID, nil
	},
}

// planProjectReference decides what a changed `project` on a project-scoped
// resource is, and returns the `project_id` the plan carries and whether the
// change replaces the resource. See referenceKind.plan.
func planProjectReference(ctx context.Context, c *client.Client, config, stateRef, stateID types.String, diags *diag.Diagnostics) (types.String, bool) {
	return projectReference.plan(ctx, c, config, stateRef, stateID, diags)
}

// plan decides what a changed reference is, and returns the id the plan
// carries and whether the change replaces the resource. An unchanged reference
// costs no request. A changed one is resolved once: the same resource spelled
// differently is an update in place that writes nothing, a different one
// replaces, and one that resolves to nothing fails the plan. An unknown
// reference replaces, since nothing can show it is the same resource, and so
// does a reference set where none was or removed where one was, which on a
// resource held in a project or an organization is a move between the two.
//
// State written before the id was recorded has none, which a refresh fills in
// and a plan without one meets: its reference then holds either what was
// written or the id the API answered, so it is resolved too, and the plan
// records the id.
func (k referenceKind) plan(ctx context.Context, c *client.Client, config, stateRef, stateID types.String, diags *diag.Diagnostics) (types.String, bool) {
	recorded := !stateID.IsNull() && !stateID.IsUnknown() && stateID.ValueString() != ""
	switch {
	case config.IsUnknown():
		return types.StringUnknown(), true
	case config.IsNull() && stateRef.IsNull():
		return types.StringNull(), false
	case config.IsNull():
		return types.StringNull(), true
	case stateRef.IsNull():
		return types.StringUnknown(), true
	case recorded && config.Equal(stateRef):
		return stateID, false
	}
	id, err := k.resolve(ctx, c, config.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root(k.attr), "Error resolving "+k.name,
			fmt.Sprintf("%q could not be resolved to %s, so the plan cannot tell another spelling of "+
				"the same %s from a different one: %s", config.ValueString(), k.noun, k.name, err))
		return stateID, false
	}
	if config.Equal(stateRef) {
		return types.StringValue(id), false
	}
	prior := stateID.ValueString()
	if !recorded {
		prior = stateRef.ValueString()
		if resolved, err := k.resolve(ctx, c, prior); err == nil {
			prior = resolved
		}
	}
	if id == prior {
		return types.StringValue(id), false
	}
	return types.StringUnknown(), true
}

// planReferences is a ModifyPlan for a resource whose references decide
// nothing else in its plan: each changed one is resolved, its id is planned,
// and the plan replaces the resource when one names a different resource. It
// reports whether one does.
func planReferences(ctx context.Context, c *client.Client, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, kinds ...referenceKind) bool {
	// Nothing to compare against on create, and nothing to plan on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return false
	}
	replace := false
	for _, k := range kinds {
		var config, stateRef, stateID types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(k.attr), &config)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(k.attr), &stateRef)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(k.partner), &stateID)...)
		if resp.Diagnostics.HasError() {
			return false
		}
		id, replaces := k.plan(ctx, c, config, stateRef, stateID, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return false
		}
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(k.partner), id)...)
		if replaces {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root(k.attr))
			replace = true
		}
	}
	return replace
}

// respellInPlace carries out an update that changes nothing but how references
// are spelled, which is what a plan from planReferences is when it does not
// replace: the API holds ids, so there is nothing to write, and state takes
// the new spelling over what it held. It reports whether the update was one;
// otherwise it does nothing and the caller writes the change.
//
// A planned value that is unknown is not a change the configuration asked for:
// at apply time a value from the configuration is known, and what is left
// unknown is a computed attribute the plan leaves to the provider, which then
// keeps the value it had.
func respellInPlace(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse, kinds ...referenceKind) bool {
	var planned, prior map[string]tftypes.Value
	if req.Plan.Raw.As(&planned) != nil || req.State.Raw.As(&prior) != nil {
		return false
	}
	spelled := map[string]bool{}
	for _, k := range kinds {
		spelled[k.attr], spelled[k.partner] = true, true
		// The id is what says the reference names the same resource, so it
		// has to be planned, and planned as it was, or recorded for the first
		// time over a state written before it was.
		id, was := planned[k.partner], prior[k.partner]
		same := id.Equal(was) || (was.IsNull() && !prior[k.attr].IsNull())
		if !id.IsFullyKnown() || !same {
			return false
		}
	}
	for name, v := range planned {
		if spelled[name] || !v.IsKnown() {
			continue
		}
		if !v.IsFullyKnown() || !v.Equal(prior[name]) {
			return false
		}
	}
	resp.State.Raw = req.State.Raw.Copy()
	for _, k := range kinds {
		for _, name := range []string{k.attr, k.partner} {
			var v types.String
			resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(name), &v)...)
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(name), v)...)
		}
	}
	return true
}

// setProjectReference writes the project the API answered into `project_id`
// and leaves `project` as written. See setReference.
func setProjectReference(ref, id *types.String, answered string) {
	setReference(ref, id, answered)
}

// setReference writes the id the API answered into the id attribute and
// leaves the reference as written. A resource read without one, an import,
// takes the id as its reference, so the attribute is never empty. An answer
// naming nothing keeps what the model holds, so a computed attribute is never
// left unknown after an apply.
func setReference(ref, id *types.String, answered string) {
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

// referenceTarget is what an API call addresses the referenced resource by:
// its id once it is known, the reference as written before then. Either
// resolves at the route, and the id still does after a rename.
func referenceTarget(ref, id types.String) string {
	if !id.IsNull() && !id.IsUnknown() && id.ValueString() != "" {
		return id.ValueString()
	}
	return ref.ValueString()
}

// resolveReference fills id from ref when the API's answer carried none, so a
// resource whose answers never name it still records the id a plan compares.
// A null reference has nothing to resolve and leaves id null.
func resolveReference(ctx context.Context, c *client.Client, k referenceKind, ref types.String, id *types.String, diags *diag.Diagnostics) {
	if !id.IsNull() && !id.IsUnknown() && id.ValueString() != "" {
		return
	}
	if ref.IsNull() || ref.IsUnknown() {
		*id = types.StringNull()
		return
	}
	resolved, err := k.resolve(ctx, c, ref.ValueString())
	if err != nil {
		diags.AddAttributeError(path.Root(k.attr), "Error resolving "+k.name,
			fmt.Sprintf("%q could not be resolved to %s: %s", ref.ValueString(), k.noun, err))
		return
	}
	*id = types.StringValue(resolved)
}

// rawStateUpgrader is a StateUpgrader that runs each step over the prior
// state's raw attributes in order. The framework does not chain upgraders:
// each is handed the state as its version wrote it and returns the current
// version's, so an upgrader from an older version lists every step since.
func rawStateUpgrader(r resource.Resource, what string, steps ...func(map[string]json.RawMessage) error) resource.StateUpgrader {
	return resource.StateUpgrader{
		StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
				resp.Diagnostics.AddError("Error reading prior "+what+" state", err.Error())
				return
			}
			for _, step := range steps {
				if err := step(raw); err != nil {
					resp.Diagnostics.AddError("Error upgrading prior "+what+" state", err.Error())
					return
				}
			}
			stepped, err := json.Marshal(raw)
			if err != nil {
				resp.Diagnostics.AddError("Error writing upgraded "+what+" state", err.Error())
				return
			}
			out, err := pruneToSchema(stepped, schemaType(ctx, r))
			if err != nil {
				resp.Diagnostics.AddError("Error writing upgraded "+what+" state", err.Error())
				return
			}
			resp.DynamicValue = &tfprotov6.DynamicValue{JSON: out}
		},
	}
}

// moveReference is an upgrade step for a state written while `old` held the
// reference: the reference attribute then holds it as written and its partner
// holds the id. The old value was what the API answered, which is an id, so it
// goes into both; the first plan that spells the reference another way
// resolves it once and keeps the resource.
func moveReference(old string, k referenceKind) func(map[string]json.RawMessage) error {
	return func(raw map[string]json.RawMessage) error {
		v, ok := raw[old]
		if !ok {
			v = json.RawMessage("null")
		}
		delete(raw, old)
		raw[k.attr], raw[k.partner] = v, v
		return nil
	}
}

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
	_ resource.ResourceWithModifyPlan  = &ProjectResource{}
)

// ProjectResource defines the resource implementation.
type ProjectResource struct {
	client *client.Client
}

// ProjectResourceModel describes the resource data model.
type ProjectResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	DisplayName    types.String `tfsdk:"display_name"`
	Org            types.String `tfsdk:"org"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Egress         types.String `tfsdk:"egress"`
	AdoptExisting  types.Bool   `tfsdk:"adopt_existing"`
	Status         types.String `tfsdk:"status"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

// NewProjectResource returns a new project resource.
func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

func (r *ProjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe project. A project maps 1:1 to a Kubernetes namespace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Project ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Project name. Doubles as the namespace identity, so changing it forces a new project.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable display name (mutable cosmetic label). Defaults to the name.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org": schema.StringAttribute{
				Description: "The organization the project belongs to, by uuid, opaque id or readable name — " +
					"whichever the configuration finds convenient. Defaults to the API key's organization. " +
					"It records the reference as written and forces nothing on its own: a different spelling " +
					"of the same organization is not a change, and only a different organization replaces " +
					"the project.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Description: "The organization's frozen id. This is what a plan compares — the project is " +
					"replaced when it changes, never when `org` is merely spelled differently.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"egress": schema.StringAttribute{
				Description: "Egress policy: \"restricted\" (default), \"https\" (TCP 443 anywhere), or \"all\" (any address, any port). TCP 25 is refused on every mode, so mail goes through a relay on 587 or 465 rather than direct to MX.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"adopt_existing": schema.BoolAttribute{
				Description: "When true, if a project with this name already exists in the target " +
					"organization, adopt it into Terraform state on create instead of failing with a 409 " +
					"conflict. Defaults to false, so create never silently takes ownership of a project it " +
					"did not create.",
				Optional: true,
			},
			"status": schema.StringAttribute{
				Description: "The project's own state: `active`, `suspended`, or `deleting`. A suspended " +
					"project keeps its resources and refuses changes to them, so a plan that reads " +
					"`active` here is a plan that can apply.",
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the project was created.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the project was last updated.",
				Computed:    true,
			},
		},
	}
}

func (r *ProjectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiReq := client.CreateProjectRequest{
		Name:        plan.Name.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
		Egress:      plan.Egress.ValueString(),
	}

	var project *client.Project
	var err error
	if org := plan.Org.ValueString(); org != "" {
		project, err = r.client.CreateProjectInOrg(ctx, org, apiReq)
	} else {
		project, err = r.client.CreateProject(ctx, apiReq)
	}
	if err != nil {
		if isConflict(err) && plan.AdoptExisting.ValueBool() {
			project, err = r.findProjectByName(ctx, plan.Org.ValueString(), plan.Name.ValueString())
			if err != nil {
				resp.Diagnostics.AddError(
					"Error adopting existing project",
					adoptErrorDetail("project", plan.Name.ValueString(), err),
				)
				return
			}
		} else {
			resp.Diagnostics.AddError("Error creating project", err.Error())
			return
		}
	}

	r.apply(&plan, project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// findProjectByName resolves a project by name, scoped to org when provided
// (project names are unique per organization). An empty org uses the API key's
// default organization via ListProjects.
func (r *ProjectResource) findProjectByName(ctx context.Context, org, name string) (*client.Project, error) {
	var projects []*client.Project
	var err error
	if org != "" {
		projects, err = r.client.ListProjectsInOrg(ctx, org)
	} else {
		projects, err = r.client.ListProjects(ctx)
	}
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Name == name {
			return p, nil
		}
	}
	return nil, fmt.Errorf("project %q is %w", name, errNotAccessible)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	project, err := r.client.GetProject(ctx, state.ID.ValueString())
	if err != nil {
		// If the project was deleted out-of-band, remove it from state.
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading project", err.Error())
		return
	}

	r.apply(&state, project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	// egress, display_name, and the operator-only caps are the mutable fields;
	// name and org force replacement.
	if plan.Egress.ValueString() != state.Egress.ValueString() {
		if _, err := r.client.UpdateProjectEgress(ctx, id, plan.Egress.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error updating project egress", err.Error())
			return
		}
	}

	if plan.DisplayName.ValueString() != state.DisplayName.ValueString() {
		if _, err := r.client.UpdateProjectDisplayName(ctx, id, plan.DisplayName.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error updating project display name", err.Error())
			return
		}
	}

	// Read the settled project and write THAT, rather than whichever call above
	// happened to run last. Every computed attribute is unknown in the plan, and
	// only a value read back from the API makes it known — so an update that
	// calls nothing here is exactly the one that must still read.
	//
	// An org respelling is that update (ADR-108): `org` is recorded as written
	// and nothing about the project changes, so neither branch above fires and
	// the plan's unknowns went to state verbatim — which Terraform refuses as
	// "provider returned invalid result object after apply", on created_at,
	// status and updated_at (fogpipe/cloud-workspace#250).
	project, err := r.client.GetProject(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading project", err.Error())
		return
	}
	r.apply(&plan, project)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.DeleteProject(ctx, state.ID.ValueString()); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			// Already deleted, nothing to do.
			return
		}
		resp.Diagnostics.AddError("Error deleting project", err.Error())
		return
	}

	// The API accepts the deletion and tears the project down behind it. Terraform
	// takes a returned Delete as "the resource is gone", so waiting here is what
	// makes that true — otherwise the next apply recreates a project whose
	// namespace and storage are still being reclaimed. The teardown continues
	// server-side regardless of what this operation does (#865).
	if err := r.client.WaitProjectDeleted(ctx, state.ID.ValueString(), 5*time.Second); err != nil {
		resp.Diagnostics.AddError("Error waiting for project deletion",
			"The deletion was accepted and is still running server-side: "+err.Error())
	}
}

// ImportState accepts a project id (UUID), a bare project name, or an
// "org/name" pair. The id is tried first; on a miss the value is resolved as a
// name (optionally org-scoped) via the list API.
func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if _, err := r.client.GetProject(ctx, id); err == nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	} else if !isNotFound(err) {
		resp.Diagnostics.AddError("Error importing project", err.Error())
		return
	}

	org, name := "", id
	if parts := strings.SplitN(id, "/", 2); len(parts) == 2 {
		org, name = parts[0], parts[1]
	}
	project, err := r.findProjectByName(ctx, org, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error importing project",
			fmt.Sprintf("%q is not a known project id, name, or org/name: %s", id, err.Error()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), project.ID)...)
}

// apply copies API-returned fields onto the model. The reference in `org` is
// left as the configuration wrote it — the organization the project is actually
// in is `organization_id`, which the API returns and a plan compares. A project
// that arrived without one (an import, or a create against the key's default
// org) gets the frozen id as its reference, so the attribute is never empty and
// the first plan that names an org converges on it.
func (r *ProjectResource) apply(m *ProjectResourceModel, project *client.Project) {
	m.OrganizationID = types.StringValue(project.OrganizationID)
	if m.Org.IsNull() || m.Org.IsUnknown() || m.Org.ValueString() == "" {
		m.Org = types.StringValue(project.OrganizationID)
	}
	m.ID = types.StringValue(project.ID)
	m.Name = types.StringValue(project.Name)
	m.DisplayName = types.StringValue(project.DisplayName)
	m.Status = types.StringValue(project.Status)
	m.Egress = types.StringValue(project.Egress)
	m.CreatedAt = types.StringValue(project.CreatedAt.String())
	m.UpdatedAt = types.StringValue(project.UpdatedAt.String())
}

// ModifyPlan decides whether a changed `org` is a changed organization. The
// attribute holds a reference a person wrote, and the platform answers to a
// uuid, an opaque id and a readable name alike — so comparing the strings makes
// a rename, or a rewrite onto the id every derivation uses, indistinguishable
// from moving the project somewhere else. It is the same project, and replacing
// it takes its apps, its databases and its buckets with it.
//
// So the spelling is resolved, once, and only when it changed: the frozen id it
// resolves to is what decides. An unchanged reference costs no request, which is
// what keeps a routine plan from needing to read the organization at all.
func (r *ProjectResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing to compare against on create, and nothing to plan on destroy.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state, config ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Org.IsNull() || config.Org.IsUnknown() {
		return
	}
	if config.Org.ValueString() == state.Org.ValueString() {
		return
	}

	org, err := r.client.GetOrg(ctx, config.Org.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("org"),
			"Error resolving organization",
			fmt.Sprintf("%q could not be resolved to an organization, so the plan cannot tell a "+
				"renamed organization from a different one: %s", config.Org.ValueString(), err),
		)
		return
	}

	if org.ID == state.OrganizationID.ValueString() {
		// Same organization, spelled differently. The new spelling is recorded
		// in place; nothing about the project moves.
		return
	}

	resp.RequiresReplace = path.Paths{path.Root("org")}
}

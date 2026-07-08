package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
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
)

// ProjectResource defines the resource implementation.
type ProjectResource struct {
	client *client.Client
}

// ProjectResourceModel describes the resource data model.
type ProjectResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Org           types.String `tfsdk:"org"`
	Egress        types.String `tfsdk:"egress"`
	Plan          types.String `tfsdk:"plan"`
	AdoptExisting types.Bool   `tfsdk:"adopt_existing"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
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
			"org": schema.StringAttribute{
				Description: "Organization (ID or name) the project belongs to. Defaults to the API key's organization. Changing it forces a new project.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"egress": schema.StringAttribute{
				Description: "Egress policy: \"restricted\" (default), \"https\", or \"all\".",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"plan": schema.StringAttribute{
				Description: "Project plan: \"starter\", \"standard\", or \"premium\".",
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
		Name:   plan.Name.ValueString(),
		Egress: plan.Egress.ValueString(),
		Plan:   plan.Plan.ValueString(),
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

	// egress and plan are the only mutable fields; name and org force replacement.
	if plan.Egress.ValueString() != state.Egress.ValueString() {
		project, err := r.client.UpdateProjectEgress(ctx, id, plan.Egress.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating project egress", err.Error())
			return
		}
		r.apply(&plan, project)
	}

	if plan.Plan.ValueString() != state.Plan.ValueString() {
		project, err := r.client.UpdateProjectPlan(ctx, id, plan.Plan.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating project plan", err.Error())
			return
		}
		r.apply(&plan, project)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteProject(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			// Already deleted, nothing to do.
			return
		}
		resp.Diagnostics.AddError("Error deleting project", err.Error())
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

// apply copies API-returned fields onto the model. The org is write-only at
// create (the API never echoes it back), so it is left untouched on the model.
func (r *ProjectResource) apply(m *ProjectResourceModel, project *client.Project) {
	m.ID = types.StringValue(project.ID)
	m.Name = types.StringValue(project.Name)
	m.Egress = types.StringValue(project.Egress)
	m.Plan = types.StringValue(project.Plan)
	m.CreatedAt = types.StringValue(project.CreatedAt.String())
	m.UpdatedAt = types.StringValue(project.UpdatedAt.String())
}

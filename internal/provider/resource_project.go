package provider

import (
	"context"
	"fmt"

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
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Org       types.String `tfsdk:"org"`
	Egress    types.String `tfsdk:"egress"`
	Plan      types.String `tfsdk:"plan"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
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
		resp.Diagnostics.AddError("Error creating project", err.Error())
		return
	}

	r.apply(&plan, project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
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

func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
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

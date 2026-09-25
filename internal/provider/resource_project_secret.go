package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &ProjectSecretResource{}
	_ resource.ResourceWithConfigure = &ProjectSecretResource{}
)

// NewProjectSecretResource returns a new project secret resource (#1069).
func NewProjectSecretResource() resource.Resource {
	return &ProjectSecretResource{}
}

// ProjectSecretResource manages one named secret in a project. The value is
// write-only: the API never returns it, and an app reads it as a file mounted
// through fpcloud_app's secret_mounts.
type ProjectSecretResource struct {
	client *client.Client
}

// ProjectSecretResourceModel describes the resource data model.
type ProjectSecretResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Name      types.String `tfsdk:"name"`
	Value     types.String `tfsdk:"value"`
	MountedBy types.List   `tfsdk:"mounted_by"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (r *ProjectSecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_secret"
}

func (r *ProjectSecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a named secret in a project. The value goes in here and never comes " +
			"back out: an app reads it as a file, mounted at a path through fpcloud_app's " +
			"secret_mounts. A database's owner credential is a secret of this kind too " +
			"(<database>-owner), created with the database and rotated by its rotate-password — " +
			"mount it, never manage it here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Secret ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "Project the secret belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Secret name, unique in the project; what a mount references.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Description: "The secret value. Write-only: the API never returns it, so what the " +
					"configuration says is what the state holds. Changing it rolls every app " +
					"mounting the secret onto the new value.",
				Required:  true,
				Sensitive: true,
			},
			"mounted_by": schema.ListAttribute{
				Description: "Apps in the project mounting this secret. The mount is the bind.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *ProjectSecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *ProjectSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProjectSecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sec, err := r.client.CreateProjectSecret(ctx, plan.ProjectID.ValueString(), plan.Name.ValueString(), plan.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating project secret", err.Error())
		return
	}
	mapProjectSecretToModel(ctx, sec, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProjectSecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sec, err := r.client.GetProjectSecret(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading project secret", err.Error())
		return
	}
	// The value never comes back; the state keeps what the configuration said.
	mapProjectSecretToModel(ctx, sec, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProjectSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ProjectSecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state ProjectSecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sec, err := r.client.UpdateProjectSecret(ctx, state.ID.ValueString(), plan.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating project secret", err.Error())
		return
	}
	plan.ID = state.ID
	mapProjectSecretToModel(ctx, sec, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectSecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteProjectSecret(ctx, state.ID.ValueString()); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting project secret", err.Error())
	}
}

// mapProjectSecretToModel fills the computed fields from the API response,
// leaving Value alone: the API never returns it.
func mapProjectSecretToModel(ctx context.Context, sec *client.ProjectSecret, m *ProjectSecretResourceModel) {
	m.ID = types.StringValue(sec.ID)
	m.ProjectID = types.StringValue(sec.ProjectID)
	m.Name = types.StringValue(sec.Name)
	mounted, _ := types.ListValueFrom(ctx, types.StringType, sec.MountedBy)
	m.MountedBy = mounted
	m.CreatedAt = types.StringValue(sec.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	m.UpdatedAt = types.StringValue(sec.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

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
	_ resource.Resource              = &RegistryVisibilityResource{}
	_ resource.ResourceWithConfigure = &RegistryVisibilityResource{}
)

// NewRegistryVisibilityResource returns a new registry repo visibility resource.
func NewRegistryVisibilityResource() resource.Resource {
	return &RegistryVisibilityResource{}
}

// RegistryVisibilityResource defines the resource implementation.
type RegistryVisibilityResource struct {
	client *client.Client
}

// RegistryVisibilityResourceModel describes the resource data model.
type RegistryVisibilityResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Repo      types.String `tfsdk:"repo"`
	Public    types.Bool   `tfsdk:"public"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (r *RegistryVisibilityResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry_visibility"
}

func (r *RegistryVisibilityResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a container registry repository's public/private visibility (ADR-013 S4). " +
			"A public repo is anonymously pullable; private is the default. platform/** repos can never " +
			"be made public — the API rejects that with a 400 regardless of caller permissions. There is " +
			"no delete endpoint: destroying this resource sets public back to false.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Synthetic ID (\"<project_id>/<repo>\") — the API has no id field for a visibility record.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The project the repository belongs to. Changing it forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repo": schema.StringAttribute{
				Description: "Repository name (project-relative). Changing it forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"public": schema.BoolAttribute{
				Description: "Whether the repository is anonymously pullable. Required — explicit, since " +
					"this controls public exposure of container images.",
				Required: true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the visibility record was created.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the visibility record was last updated.",
				Computed:    true,
			},
		},
	}
}

func (r *RegistryVisibilityResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RegistryVisibilityResource) set(ctx context.Context, projectID string, m *RegistryVisibilityResourceModel) (*client.RegistryRepoVisibility, error) {
	return r.client.SetRegistryVisibility(ctx, projectID, client.SetRegistryVisibilityRequest{
		Repo:   m.Repo.ValueString(),
		Public: m.Public.ValueBool(),
	})
}

func (r *RegistryVisibilityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RegistryVisibilityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	record, err := r.set(ctx, plan.ProjectID.ValueString(), &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting registry visibility", err.Error())
		return
	}

	r.apply(&plan, record)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RegistryVisibilityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RegistryVisibilityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No single-record GET, so list the project's visibility records and find ours.
	records, err := r.client.ListRegistryVisibility(ctx, state.ProjectID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading registry visibility", err.Error())
		return
	}

	var found *client.RegistryRepoVisibility
	for _, rec := range records {
		if rec.Repo == state.Repo.ValueString() {
			found = rec
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	r.apply(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RegistryVisibilityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RegistryVisibilityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	record, err := r.set(ctx, plan.ProjectID.ValueString(), &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating registry visibility", err.Error())
		return
	}

	r.apply(&plan, record)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RegistryVisibilityResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RegistryVisibilityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No delete route — revert to private (the implicit default for a repo with
	// no visibility record) instead.
	_, err := r.client.SetRegistryVisibility(ctx, state.ProjectID.ValueString(), client.SetRegistryVisibilityRequest{
		Repo:   state.Repo.ValueString(),
		Public: false,
	})
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error reverting registry visibility to private", err.Error())
	}
}

// ImportState imports by "<project_id>/<repo>".
func (r *RegistryVisibilityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, repo, found := strings.Cut(req.ID, "/")
	if !found {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("expected \"<project_id>/<repo>\", got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("repo"), repo)...)
}

// apply maps an API RegistryRepoVisibility response onto the model.
func (r *RegistryVisibilityResource) apply(m *RegistryVisibilityResourceModel, v *client.RegistryRepoVisibility) {
	m.ID = types.StringValue(v.ProjectID + "/" + v.Repo)
	m.ProjectID = types.StringValue(v.ProjectID)
	m.Repo = types.StringValue(v.Repo)
	m.Public = types.BoolValue(v.Public)
	m.CreatedAt = types.StringValue(v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	m.UpdatedAt = types.StringValue(v.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

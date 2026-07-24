package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &ServiceAccountResource{}
	_ resource.ResourceWithConfigure = &ServiceAccountResource{}
)

// NewServiceAccountResource returns a new service account resource.
func NewServiceAccountResource() resource.Resource {
	return &ServiceAccountResource{}
}

// ServiceAccountResource defines the resource implementation.
type ServiceAccountResource struct {
	client *client.Client
}

// ServiceAccountResourceModel describes the resource data model.
type ServiceAccountResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`
	Email       types.String `tfsdk:"email"`
	Status      types.String `tfsdk:"status"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (r *ServiceAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *ServiceAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe service account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Service account ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The project this service account belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Service account name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable display name. Mutable in place.",
				Optional:    true,
				Computed:    true,
			},
			"email": schema.StringAttribute{
				Description: "Auto-generated email address for this service account.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Current status of the service account.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the service account was created.",
				Computed:    true,
			},
		},
	}
}

func (r *ServiceAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ServiceAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sa, err := r.client.CreateServiceAccount(ctx, plan.ProjectID.ValueString(), client.CreateServiceAccountRequest{
		Name:        plan.Name.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating service account", err.Error())
		return
	}

	plan.ID = types.StringValue(sa.ID)
	plan.ProjectID = types.StringValue(sa.ProjectID)
	plan.Name = types.StringValue(sa.Name)
	plan.DisplayName = types.StringValue(sa.DisplayName)
	plan.Email = types.StringValue(sa.Email)
	plan.Status = types.StringValue(sa.Status)
	plan.CreatedAt = types.StringValue(sa.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ServiceAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List SAs in project and find ours by ID.
	accounts, err := r.client.ListServiceAccounts(ctx, state.ProjectID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading service accounts", err.Error())
		return
	}

	var found *client.ServiceAccount
	for _, sa := range accounts {
		if sa.ID == state.ID.ValueString() {
			found = sa
			break
		}
	}

	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(found.ID)
	state.ProjectID = types.StringValue(found.ProjectID)
	state.Name = types.StringValue(found.Name)
	state.DisplayName = types.StringValue(found.DisplayName)
	state.Email = types.StringValue(found.Email)
	state.Status = types.StringValue(found.Status)
	state.CreatedAt = types.StringValue(found.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ServiceAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sa, err := r.client.UpdateServiceAccountDisplayName(ctx, state.ID.ValueString(), plan.DisplayName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating service account display name", err.Error())
		return
	}

	plan.ID = types.StringValue(sa.ID)
	plan.ProjectID = types.StringValue(sa.ProjectID)
	plan.Name = types.StringValue(sa.Name)
	plan.DisplayName = types.StringValue(sa.DisplayName)
	plan.Email = types.StringValue(sa.Email)
	plan.Status = types.StringValue(sa.Status)
	plan.CreatedAt = types.StringValue(sa.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ServiceAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteServiceAccount(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting service account", err.Error())
	}
}

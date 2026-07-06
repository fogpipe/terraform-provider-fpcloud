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
	_ resource.Resource              = &ServiceAccountKeyResource{}
	_ resource.ResourceWithConfigure = &ServiceAccountKeyResource{}
)

// NewServiceAccountKeyResource returns a new service account key resource.
func NewServiceAccountKeyResource() resource.Resource {
	return &ServiceAccountKeyResource{}
}

// ServiceAccountKeyResource defines the resource implementation.
type ServiceAccountKeyResource struct {
	client *client.Client
}

// ServiceAccountKeyResourceModel describes the resource data model.
type ServiceAccountKeyResourceModel struct {
	ID               types.String `tfsdk:"id"`
	ServiceAccountID types.String `tfsdk:"service_account_id"`
	APIKey           types.String `tfsdk:"api_key"`
	Prefix           types.String `tfsdk:"prefix"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

func (r *ServiceAccountKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account_key"
}

func (r *ServiceAccountKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe service account key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Key ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_account_id": schema.StringAttribute{
				Description: "The service account this key belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"api_key": schema.StringAttribute{
				Description: "The API key (only available at creation time).",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"prefix": schema.StringAttribute{
				Description: "Key prefix for identification.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the key was created.",
				Computed:    true,
			},
		},
	}
}

func (r *ServiceAccountKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceAccountKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ServiceAccountKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.CreateServiceAccountKey(ctx, plan.ServiceAccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating service account key", err.Error())
		return
	}

	plan.ID = types.StringValue(key.ID)
	plan.ServiceAccountID = types.StringValue(key.ServiceAccountID)
	plan.APIKey = types.StringValue(key.APIKey)
	plan.Prefix = types.StringValue(key.Prefix)
	plan.CreatedAt = types.StringValue(key.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceAccountKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ServiceAccountKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	keys, err := r.client.ListServiceAccountKeys(ctx, state.ServiceAccountID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading service account keys", err.Error())
		return
	}

	var found *client.ServiceAccountKey
	for _, k := range keys {
		if k.ID == state.ID.ValueString() {
			found = k
			break
		}
	}

	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Prefix = types.StringValue(found.Prefix)
	state.CreatedAt = types.StringValue(found.CreatedAt)
	// api_key is not returned on read; keep the value from state.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceAccountKeyResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Service account key resources are immutable. Changes require replacement.",
	)
}

func (r *ServiceAccountKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ServiceAccountKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteServiceAccountKey(ctx, state.ServiceAccountID.ValueString(), state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting service account key", err.Error())
	}
}

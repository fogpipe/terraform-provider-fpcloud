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
	_ resource.Resource              = &IAMBindingResource{}
	_ resource.ResourceWithConfigure = &IAMBindingResource{}
)

// NewIAMBindingResource returns a new IAM binding resource.
func NewIAMBindingResource() resource.Resource {
	return &IAMBindingResource{}
}

// IAMBindingResource defines the resource implementation.
type IAMBindingResource struct {
	client *client.Client
}

// IAMBindingResourceModel describes the resource data model.
type IAMBindingResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Role       types.String `tfsdk:"role"`
	MemberType types.String `tfsdk:"member_type"`
	MemberID   types.String `tfsdk:"member_id"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

func (r *IAMBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_binding"
}

func (r *IAMBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IAM role binding on a Fogpipe project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Binding ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The project to bind the role on.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description: "The role to grant (owner, editor, viewer).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"member_type": schema.StringAttribute{
				Description: "The type of member (user or serviceAccount).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"member_id": schema.StringAttribute{
				Description: "The ID of the member.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the binding was created.",
				Computed:    true,
			},
		},
	}
}

func (r *IAMBindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IAMBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IAMBindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	binding, err := r.client.SetIAMBinding(ctx, plan.ProjectID.ValueString(), client.SetIAMBindingRequest{
		Role:       plan.Role.ValueString(),
		MemberType: plan.MemberType.ValueString(),
		Member:     plan.MemberID.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating IAM binding", err.Error())
		return
	}

	plan.ID = types.StringValue(binding.ID)
	plan.ProjectID = types.StringValue(binding.ResourceID)
	plan.Role = types.StringValue(binding.Role)
	plan.MemberType = types.StringValue(binding.MemberType)
	plan.MemberID = types.StringValue(binding.Member)
	plan.CreatedAt = types.StringValue(binding.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IAMBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IAMBindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bindings, err := r.client.ListIAMBindings(ctx, state.ProjectID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading IAM bindings", err.Error())
		return
	}

	var found *client.IAMBinding
	for _, b := range bindings {
		if b.ID == state.ID.ValueString() {
			found = b
			break
		}
	}

	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(found.ID)
	state.ProjectID = types.StringValue(found.ResourceID)
	state.Role = types.StringValue(found.Role)
	state.MemberType = types.StringValue(found.MemberType)
	state.MemberID = types.StringValue(found.Member)
	state.CreatedAt = types.StringValue(found.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IAMBindingResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"IAM binding resources are immutable. Changes require replacement.",
	)
}

func (r *IAMBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IAMBindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RemoveIAMBinding(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting IAM binding", err.Error())
	}
}

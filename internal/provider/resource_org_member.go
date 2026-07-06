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
	_ resource.Resource              = &OrgMemberResource{}
	_ resource.ResourceWithConfigure = &OrgMemberResource{}
)

// NewOrgMemberResource returns a new org member resource.
func NewOrgMemberResource() resource.Resource {
	return &OrgMemberResource{}
}

// OrgMemberResource defines the resource implementation.
type OrgMemberResource struct {
	client *client.Client
}

// OrgMemberResourceModel describes the resource data model.
type OrgMemberResourceModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Email          types.String `tfsdk:"email"`
	Role           types.String `tfsdk:"role"`
	UserID         types.String `tfsdk:"user_id"`
	Status         types.String `tfsdk:"status"`
}

func (r *OrgMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_member"
}

func (r *OrgMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an organization member on the Fogpipe platform.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Member record ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Description: "The organization to add the member to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				Description: "Email address of the user to invite.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description: "Role to assign (admin, member).",
				Required:    true,
			},
			"user_id": schema.StringAttribute{
				Description: "The user ID of the member (populated after invite is accepted).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Status of the membership (active, pending).",
				Computed:    true,
			},
		},
	}
}

func (r *OrgMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrgMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OrgMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	member, err := r.client.InviteOrgMember(ctx,
		plan.OrganizationID.ValueString(),
		plan.Email.ValueString(),
		plan.Role.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error inviting org member", err.Error())
		return
	}

	plan.ID = types.StringValue(member.ID)
	plan.UserID = types.StringValue(member.UserID)
	plan.Status = types.StringValue(member.Status)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrgMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members, err := r.client.ListOrgMembers(ctx, state.OrganizationID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading org members", err.Error())
		return
	}

	var found *client.OrgMember
	for _, m := range members {
		if m.ID == state.ID.ValueString() {
			found = m
			break
		}
	}

	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.UserID = types.StringValue(found.UserID)
	state.Role = types.StringValue(found.Role)
	state.Status = types.StringValue(found.Status)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrgMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OrgMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state OrgMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Role change — only supported for active members with a real user ID.
	if !state.UserID.IsNull() && !state.UserID.IsUnknown() && state.UserID.ValueString() != "" {
		err := r.client.UpdateOrgMemberRole(ctx,
			plan.OrganizationID.ValueString(),
			state.UserID.ValueString(),
			plan.Role.ValueString(),
		)
		if err != nil {
			resp.Diagnostics.AddError("Error updating org member role", err.Error())
			return
		}
	}

	plan.ID = state.ID
	plan.UserID = state.UserID
	plan.Status = state.Status

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OrgMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !state.UserID.IsNull() && !state.UserID.IsUnknown() && state.UserID.ValueString() != "" {
		err := r.client.RemoveOrgMember(ctx, state.OrganizationID.ValueString(), state.UserID.ValueString())
		if err != nil {
			if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
				return
			}
			resp.Diagnostics.AddError("Error removing org member", err.Error())
		}
	}
}

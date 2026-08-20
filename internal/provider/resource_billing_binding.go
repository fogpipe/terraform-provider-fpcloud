package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &BillingBindingResource{}
	_ resource.ResourceWithConfigure   = &BillingBindingResource{}
	_ resource.ResourceWithImportState = &BillingBindingResource{}
)

// NewBillingBindingResource returns a new billing binding resource.
func NewBillingBindingResource() resource.Resource {
	return &BillingBindingResource{}
}

// BillingBindingResource manages who may see an organization's bill.
type BillingBindingResource struct {
	client *client.Client
}

// BillingBindingResourceModel describes the resource data model.
type BillingBindingResourceModel struct {
	ID         types.String `tfsdk:"id"`
	OrgID      types.String `tfsdk:"org_id"`
	Role       types.String `tfsdk:"role"`
	MemberType types.String `tfsdk:"member_type"`
	Member     types.String `tfsdk:"member"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

func (r *BillingBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_binding"
}

func (r *BillingBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Grants billing access on a Fogpipe organization.\n\n" +
			"Billing is a SEPARATE permission axis from `fpcloud_iam_binding`'s " +
			"owner/editor/viewer ladder. An organization owner does not inherit " +
			"billing access — seeing what the organization is charged requires one " +
			"of these bindings explicitly.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Binding ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Description: "The organization whose billing this grants access to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description: "billing.viewer (read cost and invoices) or billing.admin " +
					"(that, plus granting and revoking billing access).",
				Required: true,
				// Not RequiresReplace: the API's grant is an upsert on
				// (org, member), so changing the role updates the existing
				// binding. Replacing instead would delete the only
				// billing.admin before creating its successor, which the
				// server refuses — leaving the apply stuck half-way.
			},
			"member_type": schema.StringAttribute{
				Description: "user (default) or serviceAccount.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"member": schema.StringAttribute{
				Description: "Email or identity of the member.",
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

func (r *BillingBindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BillingBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BillingBindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	b, err := r.client.GrantBillingBinding(ctx,
		plan.OrgID.ValueString(), plan.Member.ValueString(),
		plan.MemberType.ValueString(), plan.Role.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error granting billing role", err.Error())
		return
	}

	r.apply(&plan, b)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BillingBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BillingBindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bindings, err := r.client.ListBillingBindings(ctx, state.OrgID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading billing bindings", err.Error())
		return
	}

	// Matched on member, not on id: the server's grant is an upsert keyed by
	// (org, member), so a role changed outside Terraform keeps the same row and
	// the same id. Matching on id alone would report no drift.
	var found *client.BillingBinding
	for _, b := range bindings {
		if b.Member == state.Member.ValueString() {
			found = b
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

func (r *BillingBindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan BillingBindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only the role is updatable; everything else replaces. The grant is an
	// upsert, so this changes the existing binding rather than adding one.
	b, err := r.client.GrantBillingBinding(ctx,
		plan.OrgID.ValueString(), plan.Member.ValueString(),
		plan.MemberType.ValueString(), plan.Role.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating billing role", err.Error())
		return
	}

	r.apply(&plan, b)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BillingBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BillingBindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The server refuses to remove an organization's last billing.admin —
	// without one, nobody can see the bill or grant the role back. That surfaces
	// here as an error rather than being worked around: destroying the last one
	// is a state Terraform should not be able to reach silently.
	err := r.client.RevokeBillingBinding(ctx,
		state.OrgID.ValueString(), state.Member.ValueString(), state.MemberType.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error revoking billing role", err.Error())
	}
}

func (r *BillingBindingResource) apply(m *BillingBindingResourceModel, b *client.BillingBinding) {
	m.ID = types.StringValue(b.ID)
	m.OrgID = types.StringValue(b.OrgID)
	m.Role = types.StringValue(b.Role)
	m.MemberType = types.StringValue(b.MemberType)
	m.Member = types.StringValue(b.Member)
	m.CreatedAt = types.StringValue(b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

// ImportState imports a binding by an "org_id/member" identifier — the pair the
// server's grant upserts on, and what Read matches by (the id alone cannot
// detect drift, see Read). Read fills in everything else.
func (r *BillingBindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Error importing billing binding",
			fmt.Sprintf("expected an import id of the form \"org_id/member\", got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("member"), parts[1])...)
}

package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                 = &BillingBudgetResource{}
	_ resource.ResourceWithConfigure    = &BillingBudgetResource{}
	_ resource.ResourceWithImportState  = &BillingBudgetResource{}
	_ resource.ResourceWithModifyPlan   = &BillingBudgetResource{}
	_ resource.ResourceWithUpgradeState = &BillingBudgetResource{}
)

// NewBillingBudgetResource returns a new billing budget resource.
func NewBillingBudgetResource() resource.Resource {
	return &BillingBudgetResource{}
}

// BillingBudgetResource manages an organization's spend target.
type BillingBudgetResource struct {
	client *client.Client
}

// BillingBudgetResourceModel describes the resource data model.
type BillingBudgetResourceModel struct {
	Org            types.String `tfsdk:"org"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Amount         types.String `tfsdk:"amount"`
	Currency       types.String `tfsdk:"currency"`
	Thresholds     types.List   `tfsdk:"thresholds"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func (r *BillingBudgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_billing_budget"
}

func (r *BillingBudgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A spend target for a Fogpipe organization, and the percentages of it " +
			"at which the organization is told.\n\n" +
			"**A budget alerts; it never caps.** Crossing a threshold changes nothing " +
			"about what runs — no deploy is refused and no workload is stopped. To bound " +
			"what a project can actually consume, set its resource caps instead.\n\n" +
			"There is at most one budget per organization, so the organization is the id. " +
			"Managing it requires `billing.admin`, a role on the organization (`fpcloud_iam_binding` with `org`).",
		Version: 1,
		Attributes: map[string]schema.Attribute{
			"org": schema.StringAttribute{
				Description: "The organization, by name or id, this budget belongs to. It records the " +
					"reference as written: another spelling of the same organization changes nothing, and " +
					"only a different organization forces a new budget.",
				Required: true,
			},
			"organization_id": schema.StringAttribute{
				Description: "The organization's id, and so the budget's: an organization has at most one. " +
					"This is what a plan compares: the budget is replaced when it changes, never when `org` " +
					"is merely spelled differently.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"amount": schema.StringAttribute{
				Description: "Target spend for the period, as a decimal string (\"250.00\"). " +
					"A string, not a number: amounts are NUMERIC end to end because a float " +
					"sum over a month of hourly rows drifts.",
				Required: true,
			},
			"currency": schema.StringAttribute{
				Description: "Currency of the amount. Defaults to EUR.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"thresholds": schema.ListAttribute{
				Description: "Percentages of `amount` to alert at. Defaults to [50, 90, 100]. " +
					"Values above 100 are allowed and useful — 150 is how you hear about it " +
					"again once you are well past the target.",
				ElementType: types.Int64Type,
				Optional:    true,
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the budget was first set.",
				Computed:    true,
			},
		},
	}
}

func (r *BillingBudgetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BillingBudgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BillingBudgetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BillingBudgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BillingBudgetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	view, err := r.client.GetBudget(ctx, referenceTarget(state.Org, state.OrganizationID))
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading billing budget", err.Error())
		return
	}
	// A null budget is a 200, not a 404: having no budget is an ordinary state
	// of an organization that exists. Removed from state all the same — the
	// resource this state describes is gone.
	if view.Budget == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	r.apply(ctx, &state, view.Budget, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ModifyPlan decides whether a changed `org` is a different organization; see
// referenceKind.plan.
func (r *BillingBudgetResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planReferences(ctx, r.client, req, resp, orgReference)
}

func (r *BillingBudgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if respellInPlace(ctx, req, resp, orgReference) {
		return
	}
	var plan BillingBudgetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BillingBudgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BillingBudgetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Destroying the budget stops further alerts. Crossings already recorded are
	// kept server-side: they are a record of what happened, not state derived
	// from this resource.
	if err := r.client.DeleteBudget(ctx, referenceTarget(state.Org, state.OrganizationID)); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting billing budget", err.Error())
	}
}

// write sends the plan to the API. Create and Update are the same call: the
// server upserts on the organization, which has at most one budget.
func (r *BillingBudgetResource) write(ctx context.Context, m *BillingBudgetResourceModel, diags *diag.Diagnostics) {
	var thresholds []int
	if !m.Thresholds.IsNull() && !m.Thresholds.IsUnknown() {
		var vals []int64
		diags.Append(m.Thresholds.ElementsAs(ctx, &vals, false)...)
		if diags.HasError() {
			return
		}
		for _, v := range vals {
			thresholds = append(thresholds, int(v))
		}
	}

	budget, err := r.client.SetBudget(ctx, referenceTarget(m.Org, m.OrganizationID), client.SetBudgetRequest{
		Amount:     m.Amount.ValueString(),
		Currency:   m.Currency.ValueString(),
		Thresholds: thresholds,
	})
	if err != nil {
		diags.AddError("Error setting billing budget", err.Error())
		return
	}
	r.apply(ctx, m, budget, diags)
}

func (r *BillingBudgetResource) apply(ctx context.Context, m *BillingBudgetResourceModel, b *client.BillingBudget, diags *diag.Diagnostics) {
	setReference(&m.Org, &m.OrganizationID, b.OrgID)
	// The server rounds the amount to the currency's minor unit, so state holds
	// what was stored rather than what was asked for.
	m.Amount = types.StringValue(b.Amount)
	m.Currency = types.StringValue(b.Currency)
	m.CreatedAt = types.StringValue(b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

	vals := make([]int64, len(b.Thresholds))
	for i, t := range b.Thresholds {
		vals[i] = int64(t)
	}
	list, d := types.ListValueFrom(ctx, types.Int64Type, vals)
	diags.Append(d...)
	m.Thresholds = list
}

// ImportState takes the organization alone, by name or id: an organization has
// at most one budget, so the organization names it and the schema has no
// separate id.
func (r *BillingBudgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org"), req.ID)...)
}

// UpgradeState moves a state written against schema version 0, which held the
// organization in `org_id` as the id the API answered, to `org`, as written,
// beside the computed `organization_id`.
func (r *BillingBudgetResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: rawStateUpgrader(r, "billing budget", moveReference("org_id", orgReference)),
	}
}

package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &OrgSecretResource{}
	_ resource.ResourceWithConfigure = &OrgSecretResource{}
)

// NewOrgSecretResource returns a new org secret (Fogpipe Secrets Manager) resource.
func NewOrgSecretResource() resource.Resource {
	return &OrgSecretResource{}
}

// OrgSecretResource defines the resource implementation.
type OrgSecretResource struct {
	client *client.Client
}

// OrgSecretResourceModel describes the resource data model.
type OrgSecretResourceModel struct {
	ID        types.String `tfsdk:"id"`
	OrgID     types.String `tfsdk:"org_id"`
	Name      types.String `tfsdk:"name"`
	Data      types.Map    `tfsdk:"data"`
	Targets   types.List   `tfsdk:"targets"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (r *OrgSecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_secret"
}

func (r *OrgSecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe Secrets Manager bundle (ADR-028/029): an org-scoped, envelope-" +
			"encrypted named set of key/value entries, mirrored as a k8s Secret into the org vault " +
			"namespace and into each target project's namespace. Writes replace the whole bundle " +
			"(data + targets) — there is no partial update.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Secret bundle ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Description: "The organization this secret bundle belongs to. Changing it forces a new bundle.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Bundle name, unique within the org. Changing it forces a new bundle.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"data": schema.MapAttribute{
				Description: "Key/value entries in the bundle. Must be non-empty — delete the resource " +
					"to remove a bundle rather than clearing its data. Mutable in place (a write replaces " +
					"the whole map).",
				Required:    true,
				Sensitive:   true,
				ElementType: types.StringType,
			},
			"targets": schema.ListAttribute{
				Description: "Project IDs to mirror this bundle's k8s Secret into. Mutable in place.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the bundle was created.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the bundle was last updated.",
				Computed:    true,
			},
		},
	}
}

func (r *OrgSecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrgSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OrgSecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var data map[string]string
	resp.Diagnostics.Append(plan.Data.ElementsAs(ctx, &data, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	targets := stringListToSlice(ctx, plan.Targets, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.client.CreateOrgSecret(ctx, plan.OrgID.ValueString(), plan.Name.ValueString(), data, targets)
	if err != nil {
		resp.Diagnostics.AddError("Error creating org secret", err.Error())
		return
	}

	// The create response never echoes data back (write-only on write) — keep
	// the configured value instead of trusting apply's (empty) result.
	configuredData := plan.Data
	resp.Diagnostics.Append(r.apply(ctx, &plan, secret)...)
	plan.Data = configuredData
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrgSecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unlike most write-only secrets in this provider, org secrets support an
	// authenticated reveal (requires org write permission, same as this
	// resource already needs) — read the real values back so out-of-band
	// rotation is detected instead of assumed away.
	secret, err := r.client.GetOrgSecret(ctx, state.OrgID.ValueString(), state.Name.ValueString(), true)
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading org secret", err.Error())
		return
	}

	resp.Diagnostics.Append(r.apply(ctx, &state, secret)...)
	dataValue, diags := types.MapValueFrom(ctx, types.StringType, secret.Data)
	resp.Diagnostics.Append(diags...)
	state.Data = dataValue
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrgSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state OrgSecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var data map[string]string
	resp.Diagnostics.Append(plan.Data.ElementsAs(ctx, &data, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	targets := stringListToSlice(ctx, plan.Targets, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := r.client.UpdateOrgSecret(ctx, state.OrgID.ValueString(), state.Name.ValueString(), data, targets)
	if err != nil {
		resp.Diagnostics.AddError("Error updating org secret", err.Error())
		return
	}

	configuredData := plan.Data
	resp.Diagnostics.Append(r.apply(ctx, &plan, secret)...)
	plan.Data = configuredData
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OrgSecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteOrgSecret(ctx, state.OrgID.ValueString(), state.Name.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting org secret", err.Error())
	}
}

// apply maps an API OrgSecret response onto the model. data is handled by the
// caller (Data is never populated by Create/Update; Read populates it separately
// after a reveal).
func (r *OrgSecretResource) apply(ctx context.Context, m *OrgSecretResourceModel, secret *client.OrgSecret) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(secret.ID)
	m.OrgID = types.StringValue(secret.OrgID)
	m.Name = types.StringValue(secret.Name)
	targets, d := types.ListValueFrom(ctx, types.StringType, secret.Targets)
	diags.Append(d...)
	m.Targets = targets
	m.CreatedAt = types.StringValue(secret.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	m.UpdatedAt = types.StringValue(secret.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	return diags
}

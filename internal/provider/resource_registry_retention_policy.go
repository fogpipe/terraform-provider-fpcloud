package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &RegistryRetentionPolicyResource{}
	_ resource.ResourceWithConfigure = &RegistryRetentionPolicyResource{}
)

// NewRegistryRetentionPolicyResource returns a new registry retention policy resource.
func NewRegistryRetentionPolicyResource() resource.Resource {
	return &RegistryRetentionPolicyResource{}
}

// RegistryRetentionPolicyResource defines the resource implementation.
type RegistryRetentionPolicyResource struct {
	client *client.Client
}

// RegistryRetentionPolicyResourceModel describes the resource data model.
type RegistryRetentionPolicyResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Repo       types.String `tfsdk:"repo"`
	KeepLast   types.Int64  `tfsdk:"keep_last"`
	MaxAgeDays types.Int64  `tfsdk:"max_age_days"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	CreatedAt  types.String `tfsdk:"created_at"`
	UpdatedAt  types.String `tfsdk:"updated_at"`
}

func (r *RegistryRetentionPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry_retention_policy"
}

func (r *RegistryRetentionPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an auto-delete retention policy for a project's container registry repos " +
			"(ADR-038). An hourly sweeper enforces every enabled policy across the project, independent " +
			"of Terraform — this resource only manages the policy, not when enforcement fires. Deletion " +
			"is irreversible: keep_last protects the newest N tags, max_age_days deletes tags older than " +
			"N days, both together apply as AND.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Retention policy ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The project this policy applies to. Changing it forces a new policy.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repo": schema.StringAttribute{
				Description: "Repository this policy overrides (project-relative name). Empty (the " +
					"default) is the project-wide default policy, applied to any repo without its own " +
					"override. Changing it forces a new policy.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(""),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"keep_last": schema.Int64Attribute{
				Description: "Always keep the newest N tags, regardless of age. 0 = no keep-last floor.",
				Optional:    true,
				Computed:    true,
			},
			"max_age_days": schema.Int64Attribute{
				Description: "Delete tags older than N days (the newest keep_last tags are always protected). 0 = no age limit.",
				Optional:    true,
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether this policy is enforced by the hourly sweeper. Required — an " +
					"enabled policy needs keep_last and/or max_age_days set to actually delete anything.",
				Required: true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the policy was created.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the policy was last updated.",
				Computed:    true,
			},
		},
	}
}

func (r *RegistryRetentionPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RegistryRetentionPolicyResource) set(ctx context.Context, m *RegistryRetentionPolicyResourceModel) (*client.RegistryRetentionPolicy, error) {
	return r.client.SetRetentionPolicy(ctx, m.ProjectID.ValueString(), client.SetRetentionPolicyRequest{
		Repo:       m.Repo.ValueString(),
		KeepLast:   int(m.KeepLast.ValueInt64()),
		MaxAgeDays: int(m.MaxAgeDays.ValueInt64()),
		Enabled:    m.Enabled.ValueBool(),
	})
}

func (r *RegistryRetentionPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RegistryRetentionPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error creating registry retention policy", err.Error())
		return
	}

	r.apply(&plan, policy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RegistryRetentionPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RegistryRetentionPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No single-policy GET, so list the project's policies and find ours by repo.
	policies, err := r.client.ListRetentionPolicies(ctx, state.ProjectID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading registry retention policies", err.Error())
		return
	}

	var found *client.RegistryRetentionPolicy
	for _, p := range policies {
		if p.Repo == state.Repo.ValueString() {
			found = p
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

func (r *RegistryRetentionPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RegistryRetentionPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating registry retention policy", err.Error())
		return
	}

	r.apply(&plan, policy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RegistryRetentionPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RegistryRetentionPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteRetentionPolicy(ctx, state.ProjectID.ValueString(), state.Repo.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting registry retention policy", err.Error())
	}
}

// apply maps an API RegistryRetentionPolicy response onto the model.
func (r *RegistryRetentionPolicyResource) apply(m *RegistryRetentionPolicyResourceModel, p *client.RegistryRetentionPolicy) {
	m.ID = types.StringValue(p.ID)
	m.ProjectID = types.StringValue(p.ProjectID)
	m.Repo = types.StringValue(p.Repo)
	m.KeepLast = types.Int64Value(int64(p.KeepLast))
	m.MaxAgeDays = types.Int64Value(int64(p.MaxAgeDays))
	m.Enabled = types.BoolValue(p.Enabled)
	m.CreatedAt = types.StringValue(p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	m.UpdatedAt = types.StringValue(p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

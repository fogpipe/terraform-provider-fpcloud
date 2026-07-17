package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &AppConfigResource{}
	_ resource.ResourceWithConfigure = &AppConfigResource{}
)

// NewAppConfigResource returns a new app config resource.
func NewAppConfigResource() resource.Resource {
	return &AppConfigResource{}
}

// AppConfigResource defines the resource implementation.
type AppConfigResource struct {
	client *client.Client
}

// AppConfigResourceModel describes the resource data model.
type AppConfigResourceModel struct {
	ID       types.String `tfsdk:"id"`
	AppID    types.String `tfsdk:"app_id"`
	Key      types.String `tfsdk:"key"`
	Value    types.String `tfsdk:"value"`
	IsSecret types.Bool   `tfsdk:"is_secret"`
}

func (r *AppConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_config"
}

func (r *AppConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an individual configuration key-value pair for a Fogpipe application.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the config entry.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"app_id": schema.StringAttribute{
				Description: "The application ID this config belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key": schema.StringAttribute{
				Description: "The configuration key (environment variable name).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Description: "The configuration value. Marked as sensitive when is_secret is true.",
				Required:    true,
				Sensitive:   true,
			},
			"is_secret": schema.BoolAttribute{
				Description: "Whether this config value is a secret. Secrets are redacted in API responses.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *AppConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AppConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AppConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := r.client.SetConfig(
		ctx,
		plan.AppID.ValueString(),
		plan.Key.ValueString(),
		plan.Value.ValueString(),
		plan.IsSecret.ValueBool(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating app config", err.Error())
		return
	}

	// A secret's plaintext value lives only in config — the API never echoes it
	// back verbatim (it stores it encrypted), so trusting mapAppConfigToState's
	// value would make the applied state diverge from the plan. Always keep the
	// configured value for secrets.
	configuredValue := plan.Value
	mapAppConfigToState(cfg, &plan)
	if plan.IsSecret.ValueBool() {
		plan.Value = configuredValue
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AppConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	configs, err := r.client.ListConfig(ctx, state.AppID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading app config", err.Error())
		return
	}

	var found *client.AppConfig
	for _, c := range configs {
		if c.Key == state.Key.ValueString() {
			found = c
			break
		}
	}

	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	mapAppConfigToState(found, &state)

	// If the value is a secret and the API returns a redacted value,
	// preserve the value from state to avoid unnecessary diffs.
	if found.IsSecret && found.Value == "" {
		// Keep existing state value — API redacts secret values.
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AppConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AppConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := r.client.SetConfig(
		ctx,
		plan.AppID.ValueString(),
		plan.Key.ValueString(),
		plan.Value.ValueString(),
		plan.IsSecret.ValueBool(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error updating app config", err.Error())
		return
	}

	// A secret's plaintext value lives only in config — the API never echoes it
	// back verbatim (it stores it encrypted), so trusting mapAppConfigToState's
	// value would make the applied state diverge from the plan. Always keep the
	// configured value for secrets.
	configuredValue := plan.Value
	mapAppConfigToState(cfg, &plan)
	if plan.IsSecret.ValueBool() {
		plan.Value = configuredValue
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AppConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.UnsetConfig(ctx, state.AppID.ValueString(), state.Key.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting app config", err.Error())
	}
}

// mapAppConfigToState maps an API AppConfig response to the Terraform state model.
func mapAppConfigToState(cfg *client.AppConfig, state *AppConfigResourceModel) {
	state.ID = types.StringValue(cfg.ID)
	state.AppID = types.StringValue(cfg.AppID)
	state.Key = types.StringValue(cfg.Key)
	// Only update value from API if it's not redacted (non-secret, or API returned a value).
	if cfg.Value != "" || !cfg.IsSecret {
		state.Value = types.StringValue(cfg.Value)
	}
	state.IsSecret = types.BoolValue(cfg.IsSecret)
}

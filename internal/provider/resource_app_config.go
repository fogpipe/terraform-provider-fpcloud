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
	_ resource.Resource                = &AppConfigResource{}
	_ resource.ResourceWithConfigure   = &AppConfigResource{}
	_ resource.ResourceWithImportState = &AppConfigResource{}
)

// NewAppConfigResource returns a new app config resource.
func NewAppConfigResource() resource.Resource {
	return &AppConfigResource{}
}

// AppConfigResource defines the resource implementation.
type AppConfigResource struct {
	client *client.Client
}

// AppConfigResourceModel describes the resource data model. Env is plain by
// definition (#1069): a secret is an fpcloud_project_secret, mounted on the
// app as a file.
type AppConfigResourceModel struct {
	ID    types.String `tfsdk:"id"`
	AppID types.String `tfsdk:"app_id"`
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
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
				Description: "The configuration value. Env is plain: it reads back in full. " +
					"A credential belongs in fpcloud_project_secret, mounted on the app as a file.",
				Required: true,
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
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating app config", err.Error())
		return
	}

	mapAppConfigToState(cfg, &plan)
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
	)
	if err != nil {
		resp.Diagnostics.AddError("Error updating app config", err.Error())
		return
	}

	mapAppConfigToState(cfg, &plan)
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
	state.Value = types.StringValue(cfg.Value)
}

// ImportState imports a config entry by an "app_id/key" identifier — the pair
// Read keys on. Every value reads back in full: env is plain by definition.
func (r *AppConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Error importing app config",
			fmt.Sprintf("expected an import id of the form \"app_id/key\", got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), parts[1])...)
}

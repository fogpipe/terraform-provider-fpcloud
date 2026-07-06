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
	_ resource.Resource              = &WebhookResource{}
	_ resource.ResourceWithConfigure = &WebhookResource{}
)

// NewWebhookResource returns a new webhook resource.
func NewWebhookResource() resource.Resource {
	return &WebhookResource{}
}

// WebhookResource defines the resource implementation.
type WebhookResource struct {
	client *client.Client
}

// WebhookResourceModel describes the resource data model.
type WebhookResourceModel struct {
	ID            types.String `tfsdk:"id"`
	AppID         types.String `tfsdk:"app_id"`
	Repo          types.String `tfsdk:"repo"`
	Branch        types.String `tfsdk:"branch"`
	ImagePattern  types.String `tfsdk:"image_pattern"`
	WebhookURL    types.String `tfsdk:"webhook_url"`
	WebhookSecret types.String `tfsdk:"webhook_secret"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

func (r *WebhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *WebhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a GitHub webhook for auto-deploy on a Fogpipe application.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the webhook.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"app_id": schema.StringAttribute{
				Description: "The application ID this webhook belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repo": schema.StringAttribute{
				Description: "The GitHub repository in owner/repo format.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"branch": schema.StringAttribute{
				Description: "The branch to watch for pushes.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("main"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"image_pattern": schema.StringAttribute{
				Description: "The container image pattern with template variables (e.g. ghcr.io/owner/repo:{{sha}}).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"webhook_url": schema.StringAttribute{
				Description: "The URL to configure in GitHub as the webhook endpoint.",
				Computed:    true,
			},
			"webhook_secret": schema.StringAttribute{
				Description: "The HMAC secret for webhook signature verification.",
				Computed:    true,
				Sensitive:   true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the webhook is currently enabled.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "The time the webhook was created.",
				Computed:    true,
			},
		},
	}
}

func (r *WebhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *WebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, err := r.client.SetupWebhook(ctx, plan.AppID.ValueString(), client.SetupWebhookRequest{
		Repo:         plan.Repo.ValueString(),
		Branch:       plan.Branch.ValueString(),
		ImagePattern: plan.ImagePattern.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating webhook", err.Error())
		return
	}

	mapWebhookToState(wh, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, err := r.client.GetWebhook(ctx, state.AppID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading webhook", err.Error())
		return
	}

	mapWebhookToState(wh, &state)

	// The webhook secret is typically only returned on creation.
	// If the API does not return it on read, preserve the value from state.
	if wh.WebhookSecret == "" {
		// Keep existing state value.
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WebhookResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All fields are immutable (RequiresReplace), so Update should never be called.
	resp.Diagnostics.AddError(
		"Update not supported",
		"Webhook resources are immutable. Changes require replacement.",
	)
}

func (r *WebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RemoveWebhook(ctx, state.AppID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting webhook", err.Error())
	}
}

// mapWebhookToState maps an API AppWebhook response to the Terraform state model.
func mapWebhookToState(wh *client.AppWebhook, state *WebhookResourceModel) {
	state.ID = types.StringValue(wh.ID)
	state.AppID = types.StringValue(wh.AppID)
	state.Repo = types.StringValue(wh.Repo)
	state.Branch = types.StringValue(wh.Branch)
	state.ImagePattern = types.StringValue(wh.ImagePattern)
	state.WebhookURL = types.StringValue(wh.WebhookURL)
	// Only update secret if the API returned one (typically only on create).
	if wh.WebhookSecret != "" {
		state.WebhookSecret = types.StringValue(wh.WebhookSecret)
	}
	state.Enabled = types.BoolValue(wh.Enabled)
	// The AppWebhook type doesn't have a CreatedAt field; use empty if not available.
	if state.CreatedAt.IsNull() || state.CreatedAt.IsUnknown() {
		state.CreatedAt = types.StringValue("")
	}
}

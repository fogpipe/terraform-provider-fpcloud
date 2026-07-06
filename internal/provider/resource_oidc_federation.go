package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const githubActionsIssuer = "https://token.actions.githubusercontent.com"

var (
	_ resource.Resource              = &OIDCFederationResource{}
	_ resource.ResourceWithConfigure = &OIDCFederationResource{}
)

// NewOIDCFederationResource returns a new OIDC federation trust binding resource.
func NewOIDCFederationResource() resource.Resource {
	return &OIDCFederationResource{}
}

// OIDCFederationResource defines the resource implementation.
type OIDCFederationResource struct {
	client *client.Client
}

// OIDCFederationResourceModel describes the resource data model.
type OIDCFederationResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Project         types.String `tfsdk:"project"`
	Issuer          types.String `tfsdk:"issuer"`
	Audience        types.String `tfsdk:"audience"`
	SubjectPattern  types.String `tfsdk:"subject_pattern"`
	ServiceAccount  types.String `tfsdk:"service_account"`
	TokenTTLSeconds types.Int64  `tfsdk:"token_ttl_seconds"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

func (r *OIDCFederationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oidc_federation"
}

func (r *OIDCFederationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Trusts a GitHub repository (via OIDC) to authenticate as a service account " +
			"in a Fogpipe project — the repo's CI presents its OIDC token and receives a short-lived, " +
			"service-account-scoped credential, with no stored keys. Immutable: changes force replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Trust binding ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description:   "The project (ID or name) the binding belongs to.",
				Required:      true,
				PlanModifiers: replaceStr,
			},
			"issuer": schema.StringAttribute{
				Description:   "OIDC issuer URL. Defaults to GitHub Actions.",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(githubActionsIssuer),
				PlanModifiers: replaceStr,
			},
			"audience": schema.StringAttribute{
				Description:   "Audience the OIDC token must carry (an fpcloud-controlled value). Defaults to \"fpcloud\".",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("fpcloud"),
				PlanModifiers: replaceStr,
			},
			"subject_pattern": schema.StringAttribute{
				Description:   "Subject to match, e.g. \"repo:owner/name:ref:refs/tags/*\". \"*\" is a wildcard; a bare \"*\" is rejected.",
				Required:      true,
				PlanModifiers: replaceStr,
			},
			"service_account": schema.StringAttribute{
				Description:   "Service account (email or ID) the repo may assume. Must belong to the project.",
				Required:      true,
				PlanModifiers: replaceStr,
			},
			"token_ttl_seconds": schema.Int64Attribute{
				Description: "Lifetime of minted tokens, in seconds. Defaults to 900.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(900),
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the binding was created.",
				Computed:    true,
			},
		},
	}
}

func (r *OIDCFederationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OIDCFederationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OIDCFederationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	binding, err := r.client.CreateTrustBinding(ctx, plan.Project.ValueString(), client.CreateTrustBindingRequest{
		Issuer:          plan.Issuer.ValueString(),
		Audience:        plan.Audience.ValueString(),
		SubjectPattern:  plan.SubjectPattern.ValueString(),
		ServiceAccount:  plan.ServiceAccount.ValueString(),
		TokenTTLSeconds: int(plan.TokenTTLSeconds.ValueInt64()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating OIDC federation binding", err.Error())
		return
	}

	r.apply(&plan, binding)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OIDCFederationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OIDCFederationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bindings, err := r.client.ListTrustBindings(ctx, state.Project.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading OIDC federation bindings", err.Error())
		return
	}

	var found *client.TrustBinding
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

	r.apply(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OIDCFederationResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"OIDC federation bindings are immutable. Changes require replacement.",
	)
}

func (r *OIDCFederationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OIDCFederationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteTrustBinding(ctx, state.Project.ValueString(), state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting OIDC federation binding", err.Error())
	}
}

// apply copies API-returned fields onto the model. service_account is left as the
// configured value: the API returns the resolved SA id, which would drift from a
// user-supplied email. The binding is immutable, so this cannot mask a real change.
func (r *OIDCFederationResource) apply(m *OIDCFederationResourceModel, b *client.TrustBinding) {
	m.ID = types.StringValue(b.ID)
	m.Issuer = types.StringValue(b.Issuer)
	m.Audience = types.StringValue(b.Audience)
	m.SubjectPattern = types.StringValue(b.SubjectPattern)
	m.TokenTTLSeconds = types.Int64Value(int64(b.TokenTTLSeconds))
	m.CreatedAt = types.StringValue(b.CreatedAt.String())
}

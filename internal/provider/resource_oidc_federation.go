package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
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
	_ resource.Resource                = &OIDCFederationResource{}
	_ resource.ResourceWithConfigure   = &OIDCFederationResource{}
	_ resource.ResourceWithImportState = &OIDCFederationResource{}
	_ resource.ResourceWithModifyPlan  = &OIDCFederationResource{}
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
	ProjectID       types.String `tfsdk:"project_id"`
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
			"in a Fogpipe project: the repo's CI presents its OIDC token and receives a short-lived, " +
			"service-account-scoped credential, with no stored keys. Immutable: changes force replacement, " +
			"except another spelling of the same project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Trust binding ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description: "The project, by name or id, the binding belongs to. It records the reference as " +
					"written: another spelling of the same project changes nothing, and only a different " +
					"project forces a new binding.",
				Required: true,
			},
			"project_id": schema.StringAttribute{
				Description: "The project's id. This is what a plan compares: the binding is replaced " +
					"when it changes, never when `project` is merely spelled differently.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
				Description: "Subject to match, e.g. \"repo:owner/name:ref:refs/tags/*\". \"*\" is a wildcard; a bare \"*\" is rejected. " +
					"A pattern in that slug form also matches GitHub's immutable subject form (\"repo:owner@OWNER-ID/name@REPO-ID:...\"), " +
					"which newer repositories and any renamed, transferred or opted-in one carry; a pattern written with the ids matches only tokens carrying them.",
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

	// A trust binding's answer does not name its project.
	resolveReference(ctx, r.client, projectReference, plan.Project, &plan.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	binding, err := r.client.CreateTrustBinding(ctx, plan.ProjectID.ValueString(), client.CreateTrustBindingRequest{
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

	bindings, err := r.client.ListTrustBindings(ctx, referenceTarget(state.Project, state.ProjectID))
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading OIDC federation bindings", err.Error())
		return
	}
	// An import, and state written before the id was recorded, have only
	// the project as written.
	resolveReference(ctx, r.client, projectReference, state.Project, &state.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
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

// ModifyPlan decides whether a changed `project` is a different project; see
// referenceKind.plan.
func (r *OIDCFederationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planReferences(ctx, r.client, req, resp, projectReference)
}

// Update records another spelling of the project, the one change a binding
// takes in place; everything else about it replaces it.
func (r *OIDCFederationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if respellInPlace(ctx, req, resp, projectReference) {
		return
	}
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

	err := r.client.DeleteTrustBinding(ctx, referenceTarget(state.Project, state.ProjectID), state.ID.ValueString())
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

// ImportState imports a trust binding by a "project/binding_id" identifier.
// Read deliberately never touches service_account — the API returns the
// resolved service-account id, which would drift from a config that spelled it
// as an email — so import seeds it from the API once, as the resolved id.
// Without that seed the attribute would import as null and the first plan
// would propose replacement. A config adopting an imported binding should
// therefore reference the service account by id.
func (r *OIDCFederationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Error importing OIDC federation binding",
			fmt.Sprintf("expected an import id of the form \"project/binding_id\", got %q", req.ID),
		)
		return
	}
	bindings, err := r.client.ListTrustBindings(ctx, parts[0])
	if err != nil {
		resp.Diagnostics.AddError("Error importing OIDC federation binding", err.Error())
		return
	}
	var found *client.TrustBinding
	for _, b := range bindings {
		if b.ID == parts[1] {
			found = b
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError(
			"Error importing OIDC federation binding",
			fmt.Sprintf("project %q has no trust binding %q", parts[0], parts[1]),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), found.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_account"), found.ServiceAccountID)...)
}

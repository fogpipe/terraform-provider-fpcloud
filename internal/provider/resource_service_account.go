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
	_ resource.Resource                   = &ServiceAccountResource{}
	_ resource.ResourceWithConfigure      = &ServiceAccountResource{}
	_ resource.ResourceWithImportState    = &ServiceAccountResource{}
	_ resource.ResourceWithValidateConfig = &ServiceAccountResource{}
	_ resource.ResourceWithModifyPlan     = &ServiceAccountResource{}
	_ resource.ResourceWithUpgradeState   = &ServiceAccountResource{}
)

// NewServiceAccountResource returns a new service account resource.
func NewServiceAccountResource() resource.Resource {
	return &ServiceAccountResource{}
}

// ServiceAccountResource defines the resource implementation.
type ServiceAccountResource struct {
	client *client.Client
}

// ServiceAccountResourceModel describes the resource data model.
type ServiceAccountResourceModel struct {
	ID types.String `tfsdk:"id"`
	// Exactly one of Project and Org, mirroring the control
	// plane's own CHECK: a machine identity belongs to a project, or to the
	// organization itself when it outlives every project
	// (fogpipe/cloud-workspace#778).
	Project        types.String `tfsdk:"project"`
	ProjectID      types.String `tfsdk:"project_id"`
	Org            types.String `tfsdk:"org"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	DisplayName    types.String `tfsdk:"display_name"`
	Email          types.String `tfsdk:"email"`
	Status         types.String `tfsdk:"status"`
	CreatedAt      types.String `tfsdk:"created_at"`
	App            types.String `tfsdk:"app"`
}

func (r *ServiceAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *ServiceAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe service account.",
		Version:     1,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Service account ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description: "The project, by name or id, this service account belongs to. Exactly one of " +
					"`project` and `org` must be set. It records the reference as written: another spelling " +
					"of the same project changes nothing, and only a different project forces a new " +
					"service account.",
				Optional: true,
			},
			"project_id": schema.StringAttribute{
				Description: "The id of the project `project` names, null for an account the organization " +
					"holds. This is what a plan compares: the account is replaced when it changes, never " +
					"when `project` is merely spelled differently.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org": schema.StringAttribute{
				Description: "The organization, by name or id, that holds this service account directly, for " +
					"a machine identity that outlives any single project: a CI suite that creates and destroys " +
					"its own projects, or a Terraform root. Requires org administrate. Exactly one of " +
					"`project` and `org` must be set. It records the reference as written: another spelling " +
					"of the same organization changes nothing, and only a different organization forces a " +
					"new service account.",
				Optional: true,
			},
			"organization_id": schema.StringAttribute{
				Description: "The id of the organization `org` names, null for a project's account. This is " +
					"what a plan compares: the account is replaced when it changes, never when `org` is " +
					"merely spelled differently.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Service account name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable display name. Mutable in place.",
				Optional:    true,
				Computed:    true,
			},
			"email": schema.StringAttribute{
				Description: "Auto-generated email address for this service account.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Current status of the service account.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the service account was created.",
				Computed:    true,
			},
			"app": schema.StringAttribute{
				Description: "The app this account is the identity of; null for an account declared here. An app's own identity is read from the app's `service_account`, never declared (ADR-283).",
				Computed:    true,
			},
		},
	}
}

func (r *ServiceAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ValidateConfig holds the exactly-one rule the control plane enforces with a
// CHECK constraint. Without it the two Optional attributes make three
// configurations expressible and only two valid, and the invalid pair would be
// caught by the API mid-apply — after Terraform has decided what it is doing.
func (r *ServiceAccountResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg ServiceAccountResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Unknown at plan time is not absent: a value flowing from another
	// resource is not yet readable and must not be reported as missing.
	project := !cfg.Project.IsNull() || cfg.Project.IsUnknown()
	org := !cfg.Org.IsNull() || cfg.Org.IsUnknown()
	switch {
	case project && org:
		resp.Diagnostics.AddError(
			"Both project and org set",
			"A service account belongs to a project or to the organization itself, never to both. Set exactly one.",
		)
	case !project && !org:
		resp.Diagnostics.AddError(
			"Neither project nor org set",
			"A service account needs an owner. Set project for a project-scoped machine identity, or org for one the organization holds directly.",
		)
	}
}

// applyServiceAccount writes what the API returned onto the model. The owner
// ids are written as the API reports them and the owner references stay as
// configured; the owner the config omitted stays null on both, or every apply
// reports a result inconsistent with its own plan.
func applyServiceAccount(m *ServiceAccountResourceModel, sa *client.ServiceAccount) {
	m.ID = types.StringValue(sa.ID)
	setReference(&m.Project, &m.ProjectID, sa.ProjectID)
	setReference(&m.Org, &m.OrganizationID, sa.OrganizationID)
	m.Name = types.StringValue(sa.Name)
	m.DisplayName = types.StringValue(sa.DisplayName)
	m.Email = types.StringValue(sa.Email)
	m.Status = types.StringValue(sa.Status)
	m.CreatedAt = types.StringValue(sa.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	m.App = optionalString(sa.App)
}

func (r *ServiceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ServiceAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := client.CreateServiceAccountRequest{
		Name:        plan.Name.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
	}
	var (
		sa  *client.ServiceAccount
		err error
	)
	// Which owner is set picks the endpoint: the org route needs org
	// administrate, the same bar as granting a binding on the org.
	if !plan.Org.IsNull() {
		sa, err = r.client.CreateOrgServiceAccount(ctx, plan.Org.ValueString(), body)
	} else {
		sa, err = r.client.CreateServiceAccount(ctx, plan.Project.ValueString(), body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating service account", err.Error())
		return
	}

	applyServiceAccount(&plan, sa)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ServiceAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.client.GetServiceAccount(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading service account", err.Error())
		return
	}

	applyServiceAccount(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ModifyPlan decides whether a changed `project` or `org` names a different
// one; see referenceKind.plan.
func (r *ServiceAccountResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planReferences(ctx, r.client, req, resp, projectReference, orgReference)
}

func (r *ServiceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if respellInPlace(ctx, req, resp, projectReference, orgReference) {
		return
	}
	var plan, state ServiceAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sa, err := r.client.UpdateServiceAccountDisplayName(ctx, state.ID.ValueString(), plan.DisplayName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating service account display name", err.Error())
		return
	}

	applyServiceAccount(&plan, sa)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ServiceAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteServiceAccount(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting service account", err.Error())
	}
}

// ImportState imports a service account by a
// "project/<project>/<service_account_id>" or
// "org/<org>/<service_account_id>" identifier, the owner by name or id. Read looks the
// account up in its owner's listing and there is no get-by-id, so the import
// id has to name which owner — a bare "<id>/<id>" cannot, because a project
// reference and an org reference are both opaque strings and guessing wrong
// reports the account as gone rather than as misaddressed. Read fills in
// everything else.
func (r *ServiceAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 3)
	malformed := len(parts) != 3 || parts[1] == "" || parts[2] == ""
	var owner string
	if !malformed {
		switch parts[0] {
		case "project":
			owner = "project"
		case "org":
			owner = "org"
		default:
			malformed = true
		}
	}
	if malformed {
		resp.Diagnostics.AddError(
			"Error importing service account",
			fmt.Sprintf("expected an import id of the form \"project/<project>/<service_account_id>\" or \"org/<org>/<service_account_id>\", got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(owner), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}

// UpgradeState moves a state written against schema version 0, which held the
// owner in `project_id` or `organization_id` as the id the API answered, to
// `project` or `org`, as written, beside the computed id.
func (r *ServiceAccountResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: rawStateUpgrader(r, "service account",
			moveReference("project_id", projectReference),
			moveReference("organization_id", orgReference)),
	}
}

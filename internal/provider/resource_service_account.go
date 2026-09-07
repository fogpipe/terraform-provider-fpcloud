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
	// Exactly one of ProjectID and OrganizationID, mirroring the control
	// plane's own CHECK: a machine identity belongs to a project, or to the
	// organization itself when it outlives every project
	// (fogpipe/cloud-workspace#778).
	ProjectID      types.String `tfsdk:"project_id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	DisplayName    types.String `tfsdk:"display_name"`
	Email          types.String `tfsdk:"email"`
	Status         types.String `tfsdk:"status"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func (r *ServiceAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *ServiceAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe service account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Service account ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The project this service account belongs to. Exactly one of `project_id` and `organization_id` must be set.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization_id": schema.StringAttribute{
				Description: "The organization that holds this service account directly, for a machine identity that outlives any single project — a CI suite that creates and destroys its own projects, or a Terraform root. Requires org administrate. Exactly one of `project_id` and `organization_id` must be set.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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
	project := !cfg.ProjectID.IsNull() || cfg.ProjectID.IsUnknown()
	org := !cfg.OrganizationID.IsNull() || cfg.OrganizationID.IsUnknown()
	switch {
	case project && org:
		resp.Diagnostics.AddError(
			"Both project_id and organization_id set",
			"A service account belongs to a project or to the organization itself, never to both. Set exactly one.",
		)
	case !project && !org:
		resp.Diagnostics.AddError(
			"Neither project_id nor organization_id set",
			"A service account needs an owner. Set project_id for a project-scoped machine identity, or organization_id for one the organization holds directly.",
		)
	}
}

// applyServiceAccount writes what the API returned onto the model. The owner
// fields are written back as the API reports them rather than as configured,
// and an empty one becomes null rather than "" — an Optional attribute the
// config omitted must stay null, or every apply reports a result inconsistent
// with its own plan.
func applyServiceAccount(m *ServiceAccountResourceModel, sa *client.ServiceAccount) {
	m.ID = types.StringValue(sa.ID)
	m.ProjectID = optionalString(sa.ProjectID)
	m.OrganizationID = optionalString(sa.OrganizationID)
	m.Name = types.StringValue(sa.Name)
	m.DisplayName = types.StringValue(sa.DisplayName)
	m.Email = types.StringValue(sa.Email)
	m.Status = types.StringValue(sa.Status)
	m.CreatedAt = types.StringValue(sa.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
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
	if !plan.OrganizationID.IsNull() {
		sa, err = r.client.CreateOrgServiceAccount(ctx, plan.OrganizationID.ValueString(), body)
	} else {
		sa, err = r.client.CreateServiceAccount(ctx, plan.ProjectID.ValueString(), body)
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

	// The API lists machine identities per owner and there is no get-by-id,
	// so the owner in state decides which listing ours is in. An org account
	// is not in any project's listing and vice versa.
	var (
		accounts []*client.ServiceAccount
		err      error
	)
	if !state.OrganizationID.IsNull() {
		accounts, err = r.client.ListOrgServiceAccounts(ctx, state.OrganizationID.ValueString())
	} else {
		accounts, err = r.client.ListServiceAccounts(ctx, state.ProjectID.ValueString())
	}
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading service accounts", err.Error())
		return
	}

	var found *client.ServiceAccount
	for _, sa := range accounts {
		if sa.ID == state.ID.ValueString() {
			found = sa
			break
		}
	}

	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	applyServiceAccount(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
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
// "project/<project_id>/<service_account_id>" or
// "org/<organization_id>/<service_account_id>" identifier. Read looks the
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
			owner = "project_id"
		case "org":
			owner = "organization_id"
		default:
			malformed = true
		}
	}
	if malformed {
		resp.Diagnostics.AddError(
			"Error importing service account",
			fmt.Sprintf("expected an import id of the form \"project/<project_id>/<service_account_id>\" or \"org/<organization_id>/<service_account_id>\", got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(owner), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}

package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &OrgResource{}
	_ resource.ResourceWithImportState = &OrgResource{}
)

// OrgResource defines the resource implementation.
type OrgResource struct {
	client *client.Client
}

// OrgResourceModel describes the resource data model.
type OrgResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	DisplayName   types.String `tfsdk:"display_name"`
	FKEEnabled    types.Bool   `tfsdk:"fke_enabled"`
	AdoptExisting types.Bool   `tfsdk:"adopt_existing"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

// NewOrgResource returns a new organization resource.
func NewOrgResource() resource.Resource {
	return &OrgResource{}
}

func (r *OrgResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org"
}

func (r *OrgResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe organization. name is immutable (changing it forces a new " +
			"organization); display_name is mutable in place. The org is not deleted on destroy " +
			"(it is only removed from state) — the API exposes no deletion endpoint.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Organization ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Organization name (slug). Changing it forces a new organization.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable display name. Defaults to the name. Mutable in place.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"fke_enabled": schema.BoolAttribute{
				Description: "Whether this org is entitled to FKE (tenant kubeconfig) access. Mutable in " +
					"place, but the API only lets a caller with administrate rights on the platform " +
					"operator org set this — an ordinary org owner setting it themselves gets a 403; have " +
					"an operator apply it, or set it via a provider configured with operator credentials.",
				Optional: true,
				Computed: true,
			},
			"adopt_existing": schema.BoolAttribute{
				Description: "When true, if an organization with this name already exists, adopt it into " +
					"Terraform state on create instead of failing with a 409 conflict. Defaults to false, " +
					"so create never silently takes ownership of an organization it did not create.",
				Optional: true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the organization was created.",
				Computed:    true,
			},
		},
	}
}

func (r *OrgResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *OrgResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OrgResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// short_id is server-derived from the name; the provider does not expose it.
	org, err := r.client.CreateOrg(ctx, plan.Name.ValueString(), plan.DisplayName.ValueString(), "")
	if err != nil {
		if isConflict(err) && plan.AdoptExisting.ValueBool() {
			org, err = r.findOrgByName(ctx, plan.Name.ValueString())
			if err != nil {
				resp.Diagnostics.AddError(
					"Error adopting existing organization",
					adoptErrorDetail("organization", plan.Name.ValueString(), err),
				)
				return
			}
		} else {
			resp.Diagnostics.AddError("Error creating organization", err.Error())
			return
		}
	}

	// fke_enabled has no create-time API param — it's set via a follow-up PATCH,
	// which typically 403s for a non-operator caller (see the attribute's
	// description). Only make the call when the plan actually asks for it, so a
	// caller who never touches fke_enabled never hits that permission wall.
	if plan.FKEEnabled.ValueBool() {
		updated, err := r.client.UpdateOrgFKE(ctx, org.ID, true)
		if err != nil {
			resp.Diagnostics.AddError("Error enabling FKE on organization", err.Error())
			return
		}
		org = updated
	}

	r.apply(&plan, org)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// findOrgByName resolves an organization by its name (slug) via the list API.
func (r *OrgResource) findOrgByName(ctx context.Context, name string) (*client.Organization, error) {
	orgs, err := r.client.ListOrgs(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range orgs {
		if o.Name == name {
			return o, nil
		}
	}
	return nil, fmt.Errorf("organization %q is %w", name, errNotAccessible)
}

func (r *OrgResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrgResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	org, err := r.client.GetOrg(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading organization", err.Error())
		return
	}

	r.apply(&state, org)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrgResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state OrgResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var org *client.Organization

	if plan.DisplayName.ValueString() != state.DisplayName.ValueString() {
		renamed, err := r.client.UpdateOrgDisplayName(ctx, state.ID.ValueString(), plan.DisplayName.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating organization display name", err.Error())
			return
		}
		org = renamed
	}

	if plan.FKEEnabled.ValueBool() != state.FKEEnabled.ValueBool() {
		switched, err := r.client.UpdateOrgFKE(ctx, state.ID.ValueString(), plan.FKEEnabled.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Error updating organization FKE entitlement", err.Error())
			return
		}
		org = switched
	}

	if org == nil {
		// Nothing changed API-side (e.g. only adopt_existing changed, which is
		// provider-local); re-read to keep state accurate.
		current, err := r.client.GetOrg(ctx, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading organization", err.Error())
			return
		}
		org = current
	}

	r.apply(&plan, org)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OrgResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// The API exposes no org deletion endpoint. Removing the resource from state
	// (the framework does this once Delete returns without error) leaves the
	// organization in place — surface that explicitly so it is never silent.
	resp.Diagnostics.AddWarning(
		"Organization not deleted",
		"The Fogpipe API does not support deleting organizations. The organization "+
			"was removed from Terraform state but still exists on the platform. "+
			"Remove it out-of-band if required.",
	)
}

// ImportState accepts either an organization id (UUID) or an organization name.
// The id is tried first; on a miss it is resolved as a name via the list API.
func (r *OrgResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if _, err := r.client.GetOrg(ctx, id); err != nil {
		if !isNotFound(err) {
			resp.Diagnostics.AddError("Error importing organization", err.Error())
			return
		}
		org, ferr := r.findOrgByName(ctx, id)
		if ferr != nil {
			resp.Diagnostics.AddError(
				"Error importing organization",
				fmt.Sprintf("%q is not a known organization id or name: %s", id, ferr.Error()),
			)
			return
		}
		id = org.ID
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func (r *OrgResource) apply(m *OrgResourceModel, org *client.Organization) {
	m.ID = types.StringValue(org.ID)
	m.Name = types.StringValue(org.Name)
	m.DisplayName = types.StringValue(org.DisplayName)
	m.FKEEnabled = types.BoolValue(org.FKEEnabled)
	m.CreatedAt = types.StringValue(org.CreatedAt.String())
}

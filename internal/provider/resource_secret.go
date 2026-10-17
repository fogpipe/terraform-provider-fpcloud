package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &SecretResource{}
	_ resource.ResourceWithConfigure      = &SecretResource{}
	_ resource.ResourceWithMoveState      = &SecretResource{}
	_ resource.ResourceWithImportState    = &SecretResource{}
	_ resource.ResourceWithValidateConfig = &SecretResource{}
)

// NewSecretResource returns a new secret resource (#1069, ADR-287).
func NewSecretResource() resource.Resource {
	return &SecretResource{}
}

// SecretResource manages one named secret in a project or an organization. The
// value is write-only: the API never returns it, and an app reaches it through
// an env entry referencing it (fpcloud_app's secret_env), as the variable or a
// file.
type SecretResource struct {
	client *client.Client
}

// SecretResourceModel describes the resource data model.
type SecretResourceModel struct {
	ID             types.String `tfsdk:"id"`
	ProjectID      types.String `tfsdk:"project_id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Scope          types.String `tfsdk:"scope"`
	Name           types.String `tfsdk:"name"`
	Value          types.String `tfsdk:"value"`
	UsedBy         types.List   `tfsdk:"used_by"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (r *SecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *SecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a named secret in a project or an organization. The value goes in here and " +
			"never comes back out: an app reaches it through an environment variable that references it " +
			"(fpcloud_app's secret_env, or fpcloud_app_config's `secret`), as the variable's value or as a " +
			"file the variable names. An organization secret is referenced as `org/<name>` by an app in any " +
			"of the organization's projects, once the app's identity is handed it (`reader`, granted with " +
			"organization write). A database's owner credential is a project secret of this kind too " +
			"(<database>-owner), created with the database and rotated by its rotate-password; reference " +
			"it, never manage it here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Secret ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "Project the secret belongs to. Exactly one of `project_id` and `organization_id` is set.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization_id": schema.StringAttribute{
				Description: "Organization the secret belongs to, for one written once and referenced as `org/<name>` from any of its projects. Exactly one of `project_id` and `organization_id` is set.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"scope": schema.StringAttribute{
				Description: "`project` or `organization`, from which of the two ids is set.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Secret name, unique in its scope; what an env entry references (as `org/<name>` for an organization secret).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Description: "The secret value. Write-only: the API never returns it, so what the " +
					"configuration says is what the state holds. Changing it rolls every app " +
					"referencing the secret onto the new value.",
				Required:  true,
				Sensitive: true,
			},
			"used_by": schema.ListAttribute{
				Description: "Apps whose env references this secret, as `<project>/<app>` for an organization secret. The reference is the bind.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

// ValidateConfig holds the scope rule: exactly one of project_id and
// organization_id, the way the schema holds it on the server.
func (r *SecretResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg SecretResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The API refuses an empty value, and Terraform counts "" as set, so
	// without this the refusal arrives mid-apply (fogpipe/cloud-workspace#1278).
	if !cfg.Value.IsUnknown() && cfg.Value.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("value"), "A secret holds a value", "value is empty; a secret with nothing to hold is not created. Leave the resource out instead.")
	}
	if cfg.ProjectID.IsUnknown() || cfg.OrganizationID.IsUnknown() {
		return
	}
	hasProject, hasOrg := !cfg.ProjectID.IsNull(), !cfg.OrganizationID.IsNull()
	if hasProject == hasOrg {
		resp.Diagnostics.AddAttributeError(path.Root("project_id"), "A secret is in a project or in an organization", "Set exactly one of project_id and organization_id.")
	}
}

func (r *SecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var sec *client.Secret
	var err error
	if org := plan.OrganizationID.ValueString(); org != "" {
		sec, err = r.client.CreateOrgSecret(ctx, org, plan.Name.ValueString(), plan.Value.ValueString())
	} else {
		sec, err = r.client.CreateProjectSecret(ctx, plan.ProjectID.ValueString(), plan.Name.ValueString(), plan.Value.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating secret", err.Error())
		return
	}
	mapSecretToModel(ctx, sec, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sec, err := r.client.GetSecret(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading secret", err.Error())
		return
	}
	mapSecretToModel(ctx, sec, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sec, err := r.client.UpdateSecret(ctx, state.ID.ValueString(), plan.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating secret", err.Error())
		return
	}
	mapSecretToModel(ctx, sec, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSecret(ctx, state.ID.ValueString()); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting secret", err.Error())
	}
}

// ImportState imports by id. The value is write-only, so an import holds no
// value until the configuration's is applied; the first plan after an import
// writes it, which is a rotation onto the value the configuration says.
func (r *SecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// MoveState accepts a `moved` block from fpcloud_project_secret, the resource
// this one replaced (ADR-287): the state is the same shape with the project
// scope made explicit, so the move plans no change.
func (r *SecretResource) MoveState(_ context.Context) []resource.StateMover {
	return []resource.StateMover{{
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if req.SourceTypeName != "fpcloud_project_secret" {
				return
			}
			if req.SourceRawState == nil {
				resp.Diagnostics.AddError("Unable to move fpcloud_project_secret", "The source state is empty.")
				return
			}
			var src struct {
				ID        string   `json:"id"`
				ProjectID string   `json:"project_id"`
				Name      string   `json:"name"`
				Value     string   `json:"value"`
				UsedBy    []string `json:"used_by"`
				CreatedAt string   `json:"created_at"`
				UpdatedAt string   `json:"updated_at"`
			}
			if err := json.Unmarshal(req.SourceRawState.JSON, &src); err != nil {
				resp.Diagnostics.AddError("Unable to move fpcloud_project_secret", err.Error())
				return
			}
			used, diags := types.ListValueFrom(ctx, types.StringType, src.UsedBy)
			resp.Diagnostics.Append(diags...)
			target := SecretResourceModel{
				ID:             types.StringValue(src.ID),
				ProjectID:      types.StringValue(src.ProjectID),
				OrganizationID: types.StringNull(),
				Scope:          types.StringValue("project"),
				Name:           types.StringValue(src.Name),
				Value:          types.StringValue(src.Value),
				UsedBy:         used,
				CreatedAt:      types.StringValue(src.CreatedAt),
				UpdatedAt:      types.StringValue(src.UpdatedAt),
			}
			resp.Diagnostics.Append(resp.TargetState.Set(ctx, target)...)
		},
	}}
}

// mapSecretToModel copies what the API reports; the value is never read back
// and stays what the configuration holds.
func mapSecretToModel(ctx context.Context, sec *client.Secret, m *SecretResourceModel) {
	m.ID = types.StringValue(sec.ID)
	m.Scope = types.StringValue(sec.Scope)
	m.ProjectID = optionalString(sec.ProjectID)
	m.OrganizationID = optionalString(sec.OrganizationID)
	m.Name = types.StringValue(sec.Name)
	used, _ := types.ListValueFrom(ctx, types.StringType, sec.UsedBy)
	m.UsedBy = used
	m.CreatedAt = types.StringValue(sec.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	m.UpdatedAt = types.StringValue(sec.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

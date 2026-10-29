package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var (
	_ resource.Resource                   = &IAMBindingResource{}
	_ resource.ResourceWithConfigure      = &IAMBindingResource{}
	_ resource.ResourceWithImportState    = &IAMBindingResource{}
	_ resource.ResourceWithValidateConfig = &IAMBindingResource{}
	_ resource.ResourceWithUpgradeState   = &IAMBindingResource{}
	_ resource.ResourceWithMoveState      = &IAMBindingResource{}
)

// NewIAMBindingResource returns a new IAM binding resource.
func NewIAMBindingResource() resource.Resource {
	return &IAMBindingResource{}
}

// IAMBindingResource is one binding: a member, a role and a target, the target
// an organization, a project, or a resource in one of them.
type IAMBindingResource struct {
	client *client.Client
}

// IAMBindingResourceModel describes the resource data model.
type IAMBindingResourceModel struct {
	ID        types.String `tfsdk:"id"`
	OrgID     types.String `tfsdk:"org_id"`
	ProjectID types.String `tfsdk:"project_id"`
	Resource  types.String `tfsdk:"resource"`
	Role      types.String `tfsdk:"role"`
	Member    types.String `tfsdk:"member"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// splitMember takes `user:<email>` or `serviceAccount:<email>` apart, the way
// the API spells the pair.
func splitMember(member string) (memberType, id string, err error) {
	memberType, id, ok := strings.Cut(member, ":")
	if !ok || id == "" || (memberType != "user" && memberType != "serviceAccount") {
		return "", "", fmt.Errorf("member must be user:<email> or serviceAccount:<email>, got %q", member)
	}
	return memberType, id, nil
}

func joinMember(memberType, id string) string {
	if memberType == "" {
		memberType = "user"
	}
	return memberType + ":" + id
}

func (r *IAMBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_binding"
}

func (r *IAMBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		Description: "Manages an IAM binding: a member holding a role on a Fogpipe organization or project, " +
			"or, with `resource`, handed an app, database, bucket or secret in it (`app.deployer`, `database.user`, `bucket.reader` or `bucket.user`, `secret.reader`). " +
			"Billing is two roles on the organization, `billing.viewer` and `billing.admin`, which an owner holds by inclusion. " +
			"A member may hold several roles on one target; each is its own binding. " +
			"A binding to `user:<email>` is the person's invitation. " +
			"A `moved` block from `fpcloud_org_member` or `fpcloud_billing_binding` carries their state over.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Binding ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Description: "The organization the role is held on. Exactly one of `org_id` and `project_id`.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The project the role is held on. Exactly one of `org_id` and `project_id`.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource": schema.StringAttribute{
				Description: "A resource in the scope handed to the member instead of a role on the scope, as `<kind>/<id>`: " +
					"`app/<id>`, `database/<id>`, `bucket/<id>` or `secret/<id>` of the project, or `secret/<id>` for an organization's own secret. " +
					"The role is then one held on that kind, which `fpcloud iam roles` lists: `app.deployer` on an app, `database.user` on a database, `bucket.reader`, `bucket.user` or `bucket.publisher` on a bucket, `secret.reader` on a secret. Handing a resource over needs `iam.set` on it " +
					"and everything the role carries, so an owner does it.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description: "The role: `owner`, `editor`, `viewer`, a fine role such as `app.deployer`, `billing.viewer` or `billing.admin` on an organization, " +
					"or the handed role of a `resource`. `fpcloud iam roles` lists them.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"member": schema.StringAttribute{
				Description: "The member, `user:<email>` or `serviceAccount:<email>`.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the binding was created.",
				Computed:    true,
			},
		},
	}
}

// ValidateConfig holds the target to one scope: exactly one of org_id and
// project_id, and a member spelled with its type.
func (r *IAMBindingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg IAMBindingResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasOrg := !cfg.OrgID.IsNull() && !cfg.OrgID.IsUnknown()
	hasProject := !cfg.ProjectID.IsNull() && !cfg.ProjectID.IsUnknown()
	if hasOrg == hasProject && !cfg.OrgID.IsUnknown() && !cfg.ProjectID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("project_id"), "Invalid binding target",
			"A binding is held on an organization or a project: set exactly one of org_id and project_id.")
	}
	if !cfg.Member.IsNull() && !cfg.Member.IsUnknown() {
		if _, _, err := splitMember(cfg.Member.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("member"), "Invalid member", err.Error())
		}
	}
}

func (r *IAMBindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IAMBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IAMBindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	memberType, member, err := splitMember(plan.Member.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member"), "Invalid member", err.Error())
		return
	}
	body := client.SetIAMBindingRequest{
		Role:       plan.Role.ValueString(),
		MemberType: memberType,
		Member:     member,
		Resource:   plan.Resource.ValueString(),
	}
	var binding *client.IAMBinding
	if plan.ProjectID.IsNull() {
		binding, err = r.client.SetOrgIAMBinding(ctx, plan.OrgID.ValueString(), body)
	} else {
		binding, err = r.client.SetIAMBinding(ctx, plan.ProjectID.ValueString(), body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating IAM binding", err.Error())
		return
	}
	r.apply(&plan, binding)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read finds the binding in its scope's list by id. The list is the scope's
// own rows and the rows on resources in it; a project's list also carries the
// organization's rows, marked inherited, which are never this resource's.
func (r *IAMBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IAMBindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var bindings []*client.IAMBinding
	var err error
	if state.ProjectID.IsNull() || state.ProjectID.ValueString() == "" {
		bindings, err = r.client.ListOrgIAMBindings(ctx, state.OrgID.ValueString())
	} else {
		bindings, err = r.client.ListIAMBindings(ctx, state.ProjectID.ValueString())
	}
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading IAM bindings", err.Error())
		return
	}
	var found *client.IAMBinding
	for _, b := range bindings {
		if b.ID == state.ID.ValueString() && !b.Inherited {
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

func (r *IAMBindingResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"IAM binding resources are immutable. Changes require replacement.",
	)
}

func (r *IAMBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IAMBindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var err error
	if state.ProjectID.IsNull() || state.ProjectID.ValueString() == "" {
		err = r.client.RemoveOrgIAMBinding(ctx, state.OrgID.ValueString(), state.ID.ValueString())
	} else {
		err = r.client.RemoveIAMBinding(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	}
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting IAM binding", err.Error())
	}
}

// apply copies what the API reports. The scope attribute that was set stays;
// the row says which resource, if any, it is held on.
func (r *IAMBindingResource) apply(m *IAMBindingResourceModel, b *client.IAMBinding) {
	m.ID = types.StringValue(b.ID)
	m.Role = types.StringValue(b.Role)
	m.Member = types.StringValue(joinMember(b.MemberType, b.Member))
	m.CreatedAt = types.StringValue(b.CreatedAt)
	switch {
	case b.Resource != "":
		m.Resource = types.StringValue(b.Resource)
	case b.ResourceType == "organization" || b.ResourceType == "project":
		m.Resource = types.StringNull()
	}
	if b.ResourceType == "project" {
		m.ProjectID = types.StringValue(b.ResourceID)
	}
	if b.ResourceType == "organization" {
		m.OrgID = types.StringValue(b.ResourceID)
	}
}

// ImportState imports by "project/<project_id>/<binding_id>" or
// "organization/<org_id>/<binding_id>": the API lists bindings per scope and
// Read looks ours up in that list, filling in everything else, the resource it
// is held on included.
func (r *IAMBindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" || (parts[0] != "project" && parts[0] != "organization") {
		resp.Diagnostics.AddError(
			"Error importing IAM binding",
			fmt.Sprintf("expected an import id of the form \"project/<project_id>/<binding_id>\" or \"organization/<org_id>/<binding_id>\", got %q", req.ID),
		)
		return
	}
	scope := path.Root("project_id")
	if parts[0] == "organization" {
		scope = path.Root("org_id")
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, scope, parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}

// UpgradeState moves a state written against schema version 0 forward:
// `member_type` and `member_id` become one `member`, and `org_id`, which did
// not exist, is null.
func (r *IAMBindingResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {StateUpgrader: upgradeIAMBindingStateV0},
	}
}

func upgradeIAMBindingStateV0(_ context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
		resp.Diagnostics.AddError("Error reading prior IAM binding state", err.Error())
		return
	}
	var memberType, memberID string
	if v, ok := raw["member_type"]; ok {
		_ = json.Unmarshal(v, &memberType)
	}
	if v, ok := raw["member_id"]; ok {
		_ = json.Unmarshal(v, &memberID)
	}
	member, _ := json.Marshal(joinMember(memberType, memberID))
	raw["member"] = member
	delete(raw, "member_type")
	delete(raw, "member_id")
	if _, ok := raw["org_id"]; !ok {
		raw["org_id"] = json.RawMessage("null")
	}
	if _, ok := raw["resource"]; !ok {
		raw["resource"] = json.RawMessage("null")
	}
	out, err := json.Marshal(raw)
	if err != nil {
		resp.Diagnostics.AddError("Error writing upgraded IAM binding state", err.Error())
		return
	}
	resp.DynamicValue = &tfprotov6.DynamicValue{JSON: out}
}

// MoveState accepts a `moved` block from the two resources this one replaced:
// fpcloud_billing_binding, a billing role being a binding on the
// organization, and fpcloud_org_member, an organization role for a person.
// Each moves as the same triple with the organization as its target and the
// member spelled with its type.
func (r *IAMBindingResource) MoveState(_ context.Context) []resource.StateMover {
	return []resource.StateMover{{
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if req.SourceTypeName != "fpcloud_org_member" {
				return
			}
			if req.SourceRawState == nil {
				resp.Diagnostics.AddError("Unable to move fpcloud_org_member", "The source state is empty.")
				return
			}
			// The member's id was the id of the binding that held their role,
			// so the moved resource reads that binding back on its next refresh.
			var src struct {
				ID             string `json:"id"`
				OrganizationID string `json:"organization_id"`
				Email          string `json:"email"`
				Role           string `json:"role"`
			}
			if err := json.Unmarshal(req.SourceRawState.JSON, &src); err != nil {
				resp.Diagnostics.AddError("Unable to move fpcloud_org_member", err.Error())
				return
			}
			target := IAMBindingResourceModel{
				ID:        types.StringValue(src.ID),
				OrgID:     types.StringValue(src.OrganizationID),
				ProjectID: types.StringNull(),
				Resource:  types.StringNull(),
				Role:      types.StringValue(src.Role),
				Member:    types.StringValue(joinMember("user", src.Email)),
				CreatedAt: types.StringNull(),
			}
			resp.Diagnostics.Append(resp.TargetState.Set(ctx, target)...)
		},
	}, {
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if req.SourceTypeName != "fpcloud_billing_binding" {
				return
			}
			if req.SourceRawState == nil {
				resp.Diagnostics.AddError("Unable to move fpcloud_billing_binding", "The source state is empty.")
				return
			}
			var src struct {
				ID         string `json:"id"`
				OrgID      string `json:"org_id"`
				Role       string `json:"role"`
				MemberType string `json:"member_type"`
				Member     string `json:"member"`
				CreatedAt  string `json:"created_at"`
			}
			if err := json.Unmarshal(req.SourceRawState.JSON, &src); err != nil {
				resp.Diagnostics.AddError("Unable to move fpcloud_billing_binding", err.Error())
				return
			}
			target := IAMBindingResourceModel{
				ID:        types.StringValue(src.ID),
				OrgID:     types.StringValue(src.OrgID),
				ProjectID: types.StringNull(),
				Resource:  types.StringNull(),
				Role:      types.StringValue(src.Role),
				Member:    types.StringValue(joinMember(src.MemberType, src.Member)),
				CreatedAt: types.StringValue(src.CreatedAt),
			}
			resp.Diagnostics.Append(resp.TargetState.Set(ctx, target)...)
		},
	}}
}

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
	_ resource.Resource                = &IAMBindingResource{}
	_ resource.ResourceWithConfigure   = &IAMBindingResource{}
	_ resource.ResourceWithImportState = &IAMBindingResource{}
)

// NewIAMBindingResource returns a new IAM binding resource.
func NewIAMBindingResource() resource.Resource {
	return &IAMBindingResource{}
}

// IAMBindingResource defines the resource implementation.
type IAMBindingResource struct {
	client *client.Client
}

// IAMBindingResourceModel describes the resource data model.
type IAMBindingResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	Resource   types.String `tfsdk:"resource"`
	Role       types.String `tfsdk:"role"`
	MemberType types.String `tfsdk:"member_type"`
	MemberID   types.String `tfsdk:"member_id"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

// accessResource is a resource a binding is held on, spelled
// "<kind>/<id>" in the resource attribute: a database, a bucket or a secret.
type accessResource struct{ Kind, ID string }

func parseAccessResource(ref string) (*accessResource, error) {
	kind, id, ok := strings.Cut(ref, "/")
	if !ok || kind == "" || id == "" {
		return nil, fmt.Errorf("resource must be <kind>/<id> with kind database, bucket or secret, got %q", ref)
	}
	switch kind {
	case "database", "bucket", "secret":
		return &accessResource{Kind: kind, ID: id}, nil
	}
	return nil, fmt.Errorf("a binding is held on a database, a bucket or a secret, not on a %s", kind)
}

// The access routes are per kind, so each call names the client method the
// kind takes.

func (r *IAMBindingResource) setAccess(ctx context.Context, target *accessResource, body client.SetIAMBindingRequest) (*client.IAMBinding, error) {
	switch target.Kind {
	case "database":
		return r.client.SetDatabaseAccess(ctx, target.ID, body)
	case "bucket":
		return r.client.SetBucketAccess(ctx, target.ID, body)
	}
	return r.client.SetSecretAccess(ctx, target.ID, body)
}

func (r *IAMBindingResource) listAccess(ctx context.Context, target *accessResource) ([]*client.IAMBinding, error) {
	switch target.Kind {
	case "database":
		return r.client.ListDatabaseAccess(ctx, target.ID)
	case "bucket":
		return r.client.ListBucketAccess(ctx, target.ID)
	}
	return r.client.ListSecretAccess(ctx, target.ID)
}

func (r *IAMBindingResource) removeAccess(ctx context.Context, target *accessResource, bindingID string) error {
	switch target.Kind {
	case "database":
		return r.client.RemoveDatabaseAccess(ctx, target.ID, bindingID)
	case "bucket":
		return r.client.RemoveBucketAccess(ctx, target.ID, bindingID)
	}
	return r.client.RemoveSecretAccess(ctx, target.ID, bindingID)
}

func (r *IAMBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iam_binding"
}

func (r *IAMBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IAM binding: a role on a Fogpipe project, or with `resource` a database, bucket or secret handed to a member (`database.user`, `bucket.user` or `secret.reader`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Binding ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The project to bind the role on.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description: "The role to grant (owner, editor, viewer).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"member_type": schema.StringAttribute{
				Description: "The type of member (user or serviceAccount).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"member_id": schema.StringAttribute{
				Description: "The ID of the member.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource": schema.StringAttribute{
				Description: "A resource in the project handed to the member instead of a project role, as `<kind>/<id>`: `database/<id>`, `bucket/<id>` or `secret/<id>`. The role is then `database.user`, `bucket.user` or `secret.reader`. Handing a resource over needs what reading its credential needs: write on the database, administration of the bucket, write on the project for a secret.",
				Optional:    true,
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

	body := client.SetIAMBindingRequest{
		Role:       plan.Role.ValueString(),
		MemberType: plan.MemberType.ValueString(),
		Member:     plan.MemberID.ValueString(),
	}
	var binding *client.IAMBinding
	var err error
	if !plan.Resource.IsNull() && plan.Resource.ValueString() != "" {
		target, perr := parseAccessResource(plan.Resource.ValueString())
		if perr != nil {
			resp.Diagnostics.AddError("Invalid resource", perr.Error())
			return
		}
		binding, err = r.setAccess(ctx, target, body)
	} else {
		binding, err = r.client.SetIAMBinding(ctx, plan.ProjectID.ValueString(), body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating IAM binding", err.Error())
		return
	}

	plan.ID = types.StringValue(binding.ID)
	if binding.ResourceType == "project" {
		plan.ProjectID = types.StringValue(binding.ResourceID)
	}
	plan.Role = types.StringValue(binding.Role)
	plan.MemberType = types.StringValue(binding.MemberType)
	plan.MemberID = types.StringValue(binding.Member)
	plan.CreatedAt = types.StringValue(binding.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IAMBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IAMBindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var bindings []*client.IAMBinding
	var err error
	wantType := "project"
	if !state.Resource.IsNull() && state.Resource.ValueString() != "" {
		target, perr := parseAccessResource(state.Resource.ValueString())
		if perr != nil {
			resp.Diagnostics.AddError("Invalid resource", perr.Error())
			return
		}
		wantType = target.Kind
		bindings, err = r.listAccess(ctx, target)
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
		if b.ID == state.ID.ValueString() && b.ResourceType == wantType {
			found = b
			break
		}
	}

	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(found.ID)
	if found.ResourceType == "project" {
		state.ProjectID = types.StringValue(found.ResourceID)
	}
	state.Role = types.StringValue(found.Role)
	state.MemberType = types.StringValue(found.MemberType)
	state.MemberID = types.StringValue(found.Member)
	state.CreatedAt = types.StringValue(found.CreatedAt)

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
	if !state.Resource.IsNull() && state.Resource.ValueString() != "" {
		target, perr := parseAccessResource(state.Resource.ValueString())
		if perr != nil {
			resp.Diagnostics.AddError("Invalid resource", perr.Error())
			return
		}
		err = r.removeAccess(ctx, target, state.ID.ValueString())
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

// ImportState imports a project binding by "project_id/binding_id", or a
// resource binding by "project_id/<kind>/<resource_id>/binding_id" — the API
// lists bindings per project or per resource and Read looks ours up in that
// list, so the scope is needed. Read fills in everything else, including the
// member spelled the way the server canonicalized it.
func (r *IAMBindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	for _, p := range parts {
		if p == "" {
			parts = nil
			break
		}
	}
	switch len(parts) {
	case 2:
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
	case 4:
		if _, err := parseAccessResource(parts[1] + "/" + parts[2]); err != nil {
			resp.Diagnostics.AddError("Error importing IAM binding", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource"), parts[1]+"/"+parts[2])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[3])...)
	default:
		resp.Diagnostics.AddError(
			"Error importing IAM binding",
			fmt.Sprintf("expected an import id of the form \"project_id/binding_id\" or \"project_id/<kind>/<resource_id>/binding_id\", got %q", req.ID),
		)
	}
}

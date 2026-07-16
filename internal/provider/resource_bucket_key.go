package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &BucketKeyResource{}
	_ resource.ResourceWithImportState = &BucketKeyResource{}
)

// BucketKeyResource defines the resource implementation.
type BucketKeyResource struct {
	client *client.Client
}

// BucketKeyResourceModel describes the resource data model.
type BucketKeyResourceModel struct {
	ID              types.String `tfsdk:"id"`
	BucketID        types.String `tfsdk:"bucket_id"`
	Name            types.String `tfsdk:"name"`
	Read            types.Bool   `tfsdk:"read"`
	Write           types.Bool   `tfsdk:"write"`
	Owner           types.Bool   `tfsdk:"owner"`
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

// NewBucketKeyResource returns a new bucket key resource.
func NewBucketKeyResource() resource.Resource {
	return &BucketKeyResource{}
}

func (r *BucketKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bucket_key"
}

func (r *BucketKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a scoped S3 access key for a Fogpipe bucket. The secret access key is " +
			"returned once, on creation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Access key ID (the key's identity).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket_id": schema.StringAttribute{
				Description: "ID of the bucket this key is scoped to. Changing it forces a new key.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Optional human-readable name for the key. Changing it forces a new key.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"read": schema.BoolAttribute{
				Description: "Grants read access. Mutable in place.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"write": schema.BoolAttribute{
				Description: "Grants write access. Mutable in place.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"owner": schema.BoolAttribute{
				Description: "Grants owner (bucket-admin) access. Mutable in place.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"access_key_id": schema.StringAttribute{
				Description: "S3 access key ID.",
				Computed:    true,
			},
			"secret_access_key": schema.StringAttribute{
				Description: "S3 secret access key. Returned only on creation — an imported key leaves this empty.",
				Computed:    true,
				Sensitive:   true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the key was created.",
				Computed:    true,
			},
		},
	}
}

func (r *BucketKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BucketKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BucketKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.CreateBucketKey(ctx, plan.BucketID.ValueString(), client.CreateBucketKeyRequest{
		Name:  plan.Name.ValueString(),
		Read:  plan.Read.ValueBool(),
		Write: plan.Write.ValueBool(),
		Owner: plan.Owner.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating bucket key", err.Error())
		return
	}

	plan.SecretAccessKey = types.StringValue(key.SecretAccessKey)
	r.apply(&plan, key)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BucketKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// There is no get-by-id endpoint; list the bucket's keys and match by ID.
	keys, err := r.client.ListBucketKeys(ctx, state.BucketID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading bucket key", err.Error())
		return
	}

	keyID := state.AccessKeyID.ValueString()
	for _, k := range keys {
		if k.AccessKeyID == keyID {
			r.apply(&state, k) // secret_access_key preserved from state
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	// Key no longer exists on the bucket.
	resp.State.RemoveResource(ctx)
}

func (r *BucketKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state BucketKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Permissions are the only mutable fields (bucket_id/name force replacement).
	key, err := r.client.UpdateBucketKeyPermissions(ctx, state.BucketID.ValueString(), state.AccessKeyID.ValueString(), client.UpdateBucketKeyPermissionsRequest{
		Read:  plan.Read.ValueBool(),
		Write: plan.Write.ValueBool(),
		Owner: plan.Owner.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating bucket key permissions", err.Error())
		return
	}

	plan.SecretAccessKey = state.SecretAccessKey
	r.apply(&plan, key)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BucketKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteBucketKey(ctx, state.BucketID.ValueString(), state.AccessKeyID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting bucket key", err.Error())
	}
}

// ImportState imports a bucket key by a "bucket_id/access_key_id" identifier. The
// secret access key is not recoverable on import (it is returned only at creation).
func (r *BucketKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Error importing bucket key",
			fmt.Sprintf("import identifier %q must be in the form \"bucket_id/access_key_id\"", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_key_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// apply maps an API BucketKey response onto the model. secret_access_key is
// handled by the caller (populated only from the create response, else preserved).
func (r *BucketKeyResource) apply(m *BucketKeyResourceModel, key *client.BucketKey) {
	m.ID = types.StringValue(key.AccessKeyID)
	// Preserve the configured bucket_id when the API does not echo it back.
	if key.BucketID != "" {
		m.BucketID = types.StringValue(key.BucketID)
	}
	m.Name = types.StringValue(key.Name)
	m.Read = types.BoolValue(key.CanRead)
	m.Write = types.BoolValue(key.CanWrite)
	m.Owner = types.BoolValue(key.CanOwner)
	m.AccessKeyID = types.StringValue(key.AccessKeyID)
	m.CreatedAt = types.StringValue(key.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

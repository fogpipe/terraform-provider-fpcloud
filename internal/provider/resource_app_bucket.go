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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &AppBucketResource{}
	_ resource.ResourceWithConfigure   = &AppBucketResource{}
	_ resource.ResourceWithImportState = &AppBucketResource{}
)

// NewAppBucketResource returns a new app-bucket binding resource.
func NewAppBucketResource() resource.Resource {
	return &AppBucketResource{}
}

// AppBucketResource defines the resource implementation.
type AppBucketResource struct {
	client *client.Client
}

// AppBucketResourceModel describes the resource data model.
type AppBucketResourceModel struct {
	ID          types.String `tfsdk:"id"`
	AppID       types.String `tfsdk:"app_id"`
	BucketID    types.String `tfsdk:"bucket_id"`
	ReadOnly    types.Bool   `tfsdk:"read_only"`
	BucketName  types.String `tfsdk:"bucket_name"`
	Endpoint    types.String `tfsdk:"endpoint"`
	Region      types.String `tfsdk:"region"`
	AccessKeyID types.String `tfsdk:"access_key_id"`
	SecretName  types.String `tfsdk:"secret_name"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (r *AppBucketResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_bucket"
}

func (r *AppBucketResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Binds a managed bucket to an app, injecting the bucket's S3_*/AWS_* " +
			"credentials into the app's pod via a k8s Secret (envFrom). The secret access " +
			"key is never returned. Every attribute forces a new binding when changed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Binding identifier in the form \"app_id/bucket_id\".",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"app_id": schema.StringAttribute{
				Description: "ID of the app to bind the bucket to. Changing it forces a new binding.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bucket_id": schema.StringAttribute{
				Description: "ID of the bucket to bind. Changing it forces a new binding.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"read_only": schema.BoolAttribute{
				Description: "Bind with a read-only scoped key. Changing it rebinds (forces a new binding).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"bucket_name": schema.StringAttribute{
				Description: "Name of the bound bucket.",
				Computed:    true,
			},
			"endpoint": schema.StringAttribute{
				Description: "S3 endpoint URL injected into the app.",
				Computed:    true,
			},
			"region": schema.StringAttribute{
				Description: "S3 region injected into the app.",
				Computed:    true,
			},
			"access_key_id": schema.StringAttribute{
				Description: "S3 access key ID injected into the app.",
				Computed:    true,
			},
			"secret_name": schema.StringAttribute{
				Description: "Name of the k8s Secret holding the injected credentials.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the binding was created.",
				Computed:    true,
			},
		},
	}
}

func (r *AppBucketResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AppBucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AppBucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	binding, err := r.client.BindAppBucket(ctx, plan.AppID.ValueString(), plan.BucketID.ValueString(), plan.ReadOnly.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error binding bucket to app", err.Error())
		return
	}

	r.apply(&plan, binding)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppBucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AppBucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bindings, err := r.client.ListAppBuckets(ctx, state.AppID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			// The app itself is gone; the binding cannot exist.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading app bucket bindings", err.Error())
		return
	}

	bucketID := state.BucketID.ValueString()
	for _, b := range bindings {
		if b.BucketID == bucketID {
			r.apply(&state, b)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	// Binding no longer exists on the app.
	resp.State.RemoveResource(ctx)
}

// Update is a no-op: every attribute forces replacement, so the framework never
// calls this with a real change. It only re-persists the plan to satisfy the
// resource.Resource interface.
func (r *AppBucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AppBucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppBucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AppBucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.UnbindAppBucket(ctx, state.AppID.ValueString(), state.BucketID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error unbinding bucket from app", err.Error())
	}
}

// ImportState imports an app-bucket binding by an "app_id/bucket_id" identifier.
func (r *AppBucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Error importing app bucket binding",
			fmt.Sprintf("import identifier %q must be in the form \"app_id/bucket_id\"", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// apply maps an API AppBucketBinding response onto the model. app_id/bucket_id are
// preserved from the plan/state when the API does not echo them back so a required
// field is never blanked.
func (r *AppBucketResource) apply(m *AppBucketResourceModel, b *client.AppBucketBinding) {
	if b.AppID != "" {
		m.AppID = types.StringValue(b.AppID)
	}
	if b.BucketID != "" {
		m.BucketID = types.StringValue(b.BucketID)
	}
	m.ReadOnly = types.BoolValue(b.ReadOnly)
	m.BucketName = types.StringValue(b.BucketName)
	m.Endpoint = types.StringValue(b.Endpoint)
	m.Region = types.StringValue(b.Region)
	m.AccessKeyID = types.StringValue(b.AccessKeyID)
	m.SecretName = types.StringValue(b.SecretName)
	m.CreatedAt = types.StringValue(b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	m.ID = types.StringValue(m.AppID.ValueString() + "/" + m.BucketID.ValueString())
}

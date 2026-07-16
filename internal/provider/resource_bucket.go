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
	_ resource.Resource                = &BucketResource{}
	_ resource.ResourceWithImportState = &BucketResource{}
)

// BucketResource defines the resource implementation.
type BucketResource struct {
	client *client.Client
}

// BucketResourceModel describes the resource data model.
type BucketResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Project         types.String `tfsdk:"project"`
	Name            types.String `tfsdk:"name"`
	QuotaMaxSize    types.Int64  `tfsdk:"quota_max_size"`
	QuotaMaxObjects types.Int64  `tfsdk:"quota_max_objects"`
	Endpoint        types.String `tfsdk:"endpoint"`
	Region          types.String `tfsdk:"region"`
	Status          types.String `tfsdk:"status"`
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
}

// NewBucketResource returns a new bucket resource.
func NewBucketResource() resource.Resource {
	return &BucketResource{}
}

func (r *BucketResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bucket"
}

func (r *BucketResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe S3-compatible object-storage bucket (backed by Garage). The " +
			"credentials for the bucket's initial access key are returned once, on creation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Bucket ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description: "ID of the project this bucket belongs to. Changing it forces a new bucket.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Bucket name. Changing it forces a new bucket.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"quota_max_size": schema.Int64Attribute{
				Description: "Maximum total size in bytes (0 = unlimited; unset = the server default). Mutable in place.",
				Optional:    true,
				Computed:    true,
			},
			"quota_max_objects": schema.Int64Attribute{
				Description: "Maximum number of objects (0 = unlimited; unset = the server default). Mutable in place.",
				Optional:    true,
				Computed:    true,
			},
			"endpoint": schema.StringAttribute{
				Description: "S3 endpoint URL for the bucket.",
				Computed:    true,
			},
			"region": schema.StringAttribute{
				Description: "S3 region for the bucket.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Current status of the bucket.",
				Computed:    true,
			},
			"access_key_id": schema.StringAttribute{
				Description: "S3 access key ID for the bucket's initial access key.",
				Computed:    true,
			},
			"secret_access_key": schema.StringAttribute{
				Description: "S3 secret access key for the bucket's initial access key. Returned only " +
					"on creation — an imported bucket leaves this empty.",
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}

func (r *BucketResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucket, err := r.client.CreateBucket(ctx, plan.Project.ValueString(), client.CreateBucketRequest{
		Name:            plan.Name.ValueString(),
		QuotaMaxSize:    plan.QuotaMaxSize.ValueInt64(),
		QuotaMaxObjects: plan.QuotaMaxObjects.ValueInt64(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating bucket", err.Error())
		return
	}

	// The one-time secret access key is only present on the create response.
	plan.SecretAccessKey = types.StringValue(bucket.SecretAccessKey)
	r.apply(&plan, bucket)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucket, err := r.client.GetBucket(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading bucket", err.Error())
		return
	}

	// secret_access_key is never returned by Get — it is preserved from state.
	r.apply(&state, bucket)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *BucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state BucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Quotas are the only mutable fields (project/name force replacement).
	bucket, err := r.client.SetBucketQuota(ctx, state.ID.ValueString(), plan.QuotaMaxSize.ValueInt64(), plan.QuotaMaxObjects.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error updating bucket quota", err.Error())
		return
	}

	// SetBucketQuota does not return the secret — preserve it from prior state.
	plan.SecretAccessKey = state.SecretAccessKey
	r.apply(&plan, bucket)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteBucket(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok {
			if apiErr.StatusCode == 404 {
				return
			}
			if apiErr.StatusCode == 409 {
				resp.Diagnostics.AddError(
					"Bucket not empty",
					fmt.Sprintf("Bucket %q cannot be deleted while it still holds objects. "+
						"Empty the bucket first, then destroy it. (%s)", state.Name.ValueString(), apiErr.Error()),
				)
				return
			}
		}
		resp.Diagnostics.AddError("Error deleting bucket", err.Error())
	}
}

// ImportState imports a bucket by its ID. The secret access key is not
// recoverable on import (it is returned only at creation).
func (r *BucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// apply maps an API Bucket response onto the model. secret_access_key is handled
// by the caller (populated only from the create response, else preserved).
func (r *BucketResource) apply(m *BucketResourceModel, bucket *client.Bucket) {
	m.ID = types.StringValue(bucket.ID)
	// Preserve the configured project when the API does not echo it back, so a
	// required-but-unreturned field never flips to "" after apply.
	if bucket.ProjectID != "" {
		m.Project = types.StringValue(bucket.ProjectID)
	}
	m.Name = types.StringValue(bucket.Name)
	m.QuotaMaxSize = types.Int64Value(bucket.QuotaMaxSize)
	m.QuotaMaxObjects = types.Int64Value(bucket.QuotaMaxObjects)
	m.Endpoint = types.StringValue(bucket.Endpoint)
	m.Region = types.StringValue(bucket.Region)
	m.Status = types.StringValue(bucket.Status)
	m.AccessKeyID = types.StringValue(bucket.AccessKeyID)
}

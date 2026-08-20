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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &BucketLifecycleRuleResource{}
	_ resource.ResourceWithConfigure   = &BucketLifecycleRuleResource{}
	_ resource.ResourceWithImportState = &BucketLifecycleRuleResource{}
)

// NewBucketLifecycleRuleResource returns a new bucket lifecycle rule resource.
func NewBucketLifecycleRuleResource() resource.Resource {
	return &BucketLifecycleRuleResource{}
}

// BucketLifecycleRuleResource defines the resource implementation.
type BucketLifecycleRuleResource struct {
	client *client.Client
}

// BucketLifecycleRuleResourceModel describes the resource data model.
type BucketLifecycleRuleResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	BucketID                  types.String `tfsdk:"bucket_id"`
	Prefix                    types.String `tfsdk:"prefix"`
	ExpireDays                types.Int64  `tfsdk:"expire_days"`
	AbortIncompleteUploadDays types.Int64  `tfsdk:"abort_incomplete_upload_days"`
	CreatedAt                 types.String `tfsdk:"created_at"`
	UpdatedAt                 types.String `tfsdk:"updated_at"`
}

func (r *BucketLifecycleRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bucket_lifecycle_rule"
}

func (r *BucketLifecycleRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Expires a bucket's objects by age. The object store deletes objects under " +
			"`prefix` once they are `expire_days` old, so derived artefacts (renders, thumbnails, " +
			"exports) do not accumulate forever. A rule is keyed by its prefix, so one bucket can " +
			"expire one prefix while everything alongside it is kept.\n\n" +
			"Expiry is by AGE, not by whether anything still references the object: an object old " +
			"enough is deleted even if it is the current version of something. Only apply this to " +
			"content you can regenerate — it is cache eviction, not backup retention.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Lifecycle rule ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket_id": schema.StringAttribute{
				Description: "The bucket this rule applies to. Changing it forces a new rule.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"prefix": schema.StringAttribute{
				Description: "Key prefix the rule covers (e.g. `renders/`). Empty (the default) covers " +
					"the whole bucket.\n\n" +
					"The prefix is the rule's blast radius, not a label on it. It is the rule's key, " +
					"so editing it does not adjust the existing rule — it destroys that rule and " +
					"creates a different one over whatever the new prefix matches. Widening " +
					"`renders/` to `\"\"` expires the entire bucket at that age, not renders plus a " +
					"little more. The plan will say `forces replacement`; it cannot say what the new " +
					"prefix now covers, so check that yourself when the bucket also holds data you " +
					"cannot rebuild.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(""),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"expire_days": schema.Int64Attribute{
				Description: "Delete objects older than N days. 0 = no expiry.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"abort_incomplete_upload_days": schema.Int64Attribute{
				Description: "Abort multipart uploads left incomplete for N days, reclaiming their " +
					"parts. 0 = never abort.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(0),
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the rule was created.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the rule was last updated.",
				Computed:    true,
			},
		},
	}
}

func (r *BucketLifecycleRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BucketLifecycleRuleResource) set(ctx context.Context, m *BucketLifecycleRuleResourceModel) (*client.BucketLifecycleRule, error) {
	return r.client.SetBucketLifecycleRule(ctx, m.BucketID.ValueString(), client.SetBucketLifecycleRuleRequest{
		Prefix:                    m.Prefix.ValueString(),
		ExpireDays:                int(m.ExpireDays.ValueInt64()),
		AbortIncompleteUploadDays: int(m.AbortIncompleteUploadDays.ValueInt64()),
	})
}

func (r *BucketLifecycleRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BucketLifecycleRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error creating bucket lifecycle rule", err.Error())
		return
	}

	r.apply(&plan, rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketLifecycleRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BucketLifecycleRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No single-rule GET, so list the bucket's rules and find ours by prefix.
	rules, err := r.client.ListBucketLifecycleRules(ctx, state.BucketID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading bucket lifecycle rules", err.Error())
		return
	}

	var found *client.BucketLifecycleRule
	for _, rule := range rules {
		if rule.Prefix == state.Prefix.ValueString() {
			found = rule
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

func (r *BucketLifecycleRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan BucketLifecycleRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating bucket lifecycle rule", err.Error())
		return
	}

	r.apply(&plan, rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketLifecycleRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BucketLifecycleRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Removes only this prefix's rule — every other rule on the bucket stays.
	err := r.client.DeleteBucketLifecycleRule(ctx, state.BucketID.ValueString(), state.Prefix.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting bucket lifecycle rule", err.Error())
	}
}

// apply maps an API BucketLifecycleRule response onto the model.
func (r *BucketLifecycleRuleResource) apply(m *BucketLifecycleRuleResourceModel, rule *client.BucketLifecycleRule) {
	m.ID = types.StringValue(rule.ID)
	m.BucketID = types.StringValue(rule.BucketID)
	m.Prefix = types.StringValue(rule.Prefix)
	m.ExpireDays = types.Int64Value(int64(rule.ExpireDays))
	m.AbortIncompleteUploadDays = types.Int64Value(int64(rule.AbortIncompleteUploadDays))
	m.CreatedAt = types.StringValue(rule.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	m.UpdatedAt = types.StringValue(rule.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

// ImportState imports a rule by "bucket_id" (the whole-bucket rule, whose
// prefix is empty) or "bucket_id/prefix". Read looks the rule up by prefix
// within the bucket, and a prefix may contain slashes of its own, so everything
// after the first "/" is the prefix. Read fills in everything else.
func (r *BucketLifecycleRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if parts[0] == "" {
		resp.Diagnostics.AddError(
			"Error importing bucket lifecycle rule",
			fmt.Sprintf("expected an import id of the form \"bucket_id\" or \"bucket_id/prefix\", got %q", req.ID),
		)
		return
	}
	prefix := ""
	if len(parts) == 2 {
		prefix = parts[1]
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("prefix"), prefix)...)
}

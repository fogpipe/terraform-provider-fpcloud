package provider

import (
	"context"
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
	_ resource.Resource                = &BucketCORSResource{}
	_ resource.ResourceWithConfigure   = &BucketCORSResource{}
	_ resource.ResourceWithImportState = &BucketCORSResource{}
)

// NewBucketCORSResource returns a new bucket CORS configuration resource.
func NewBucketCORSResource() resource.Resource {
	return &BucketCORSResource{}
}

// BucketCORSResource defines the resource implementation.
type BucketCORSResource struct {
	client *client.Client
}

// BucketCORSResourceModel describes the resource data model.
//
// One resource per bucket holding the whole rule list, the same shape
// `aws_s3_bucket_cors_configuration` and `cloudflare_r2_bucket_cors` take —
// because that is what the underlying S3 call is. A resource per rule would
// promise an independence that does not exist: writing one rule replaces the
// document, so two such resources would each silently delete the other's.
type BucketCORSResourceModel struct {
	ID       types.String       `tfsdk:"id"`
	BucketID types.String       `tfsdk:"bucket_id"`
	Rules    []BucketCORSRuleTF `tfsdk:"rule"`
}

// BucketCORSRuleTF is one rule in that configuration.
type BucketCORSRuleTF struct {
	AllowedOrigins []types.String `tfsdk:"allowed_origins"`
	AllowedMethods []types.String `tfsdk:"allowed_methods"`
	AllowedHeaders []types.String `tfsdk:"allowed_headers"`
	ExposeHeaders  []types.String `tfsdk:"expose_headers"`
	MaxAgeSeconds  types.Int64    `tfsdk:"max_age_seconds"`
}

func (r *BucketCORSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bucket_cors"
}

func (r *BucketCORSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lets a browser call a bucket directly. Until an origin is allowed here, a page " +
			"on your own domain cannot upload to or read from the bucket: the browser sends an " +
			"OPTIONS preflight first and refuses the real request if the answer does not name the " +
			"origin.\n\n" +
			"A presigned URL does not remove the need for this. The preflight carries no signature, " +
			"so it is answered before any credential is considered — a correctly signed upload " +
			"still fails without a rule, and the browser reports it as a CORS error rather than as " +
			"anything about permissions. The two answer different questions: the signature says who " +
			"you are, this says the browser may make the request at all.\n\n" +
			"You only need this when the BROWSER talks to the bucket. An app uploading through its " +
			"own backend makes no cross-origin request and needs nothing here.\n\n" +
			"This resource owns the bucket's whole CORS configuration — declare one per bucket. " +
			"Two of them against the same bucket will each overwrite the other on every apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The bucket's ID. A bucket has exactly one CORS configuration.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket_id": schema.StringAttribute{
				Description: "The bucket this configuration applies to. Changing it forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rule": schema.ListNestedAttribute{
				Description: "The cross-origin rules, in order. Order is significant: the object store " +
					"answers a preflight with the first rule whose origin matches, so a broad rule " +
					"above a narrow one hides it.",
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"allowed_origins": schema.ListAttribute{
							Description: "Origins allowed to call the bucket, each `scheme://host[:port]` with no " +
								"trailing slash — `example.com` matches nothing, because the browser sends " +
								"`https://example.com`. `*` allows any origin, which is weaker than it sounds " +
								"here: bucket requests carry no cookies, so there is no ambient authority for a " +
								"hostile origin to ride on and CORS can only say the browser may READ the " +
								"response, never that the caller may access anything.",
							Required:    true,
							ElementType: types.StringType,
						},
						"allowed_methods": schema.ListAttribute{
							Description: "HTTP methods the origin may use: GET, PUT, HEAD, POST, DELETE. A rule " +
								"naming no method allows nothing.",
							Required:    true,
							ElementType: types.StringType,
						},
						"allowed_headers": schema.ListAttribute{
							Description: "Request headers the browser may send, matched against the preflight's " +
								"`Access-Control-Request-Headers`. `*` allows any. A presigned upload sends " +
								"content-type and the signed headers, so `*` is the usual answer.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"expose_headers": schema.ListAttribute{
							Description: "Response headers the page is allowed to read. Easy to omit and hard to " +
								"diagnose: without `ETag` an upload succeeds and the page cannot see what it " +
								"uploaded.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"max_age_seconds": schema.Int64Attribute{
							Description: "How long the browser may cache the preflight, up to 86400. Omitted " +
								"leaves it to the browser, which means a preflight per request.",
							Optional: true,
						},
					},
				},
			},
		},
	}
}

func (r *BucketCORSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BucketCORSResource) set(ctx context.Context, m *BucketCORSResourceModel) ([]*client.BucketCORSRule, error) {
	rules := make([]client.BucketCORSRuleRequest, 0, len(m.Rules))
	for _, rule := range m.Rules {
		rules = append(rules, client.BucketCORSRuleRequest{
			AllowedOrigins: stringValues(rule.AllowedOrigins),
			AllowedMethods: stringValues(rule.AllowedMethods),
			AllowedHeaders: stringValues(rule.AllowedHeaders),
			ExposeHeaders:  stringValues(rule.ExposeHeaders),
			MaxAgeSeconds:  int(rule.MaxAgeSeconds.ValueInt64()),
		})
	}
	return r.client.SetBucketCORSRules(ctx, m.BucketID.ValueString(), client.SetBucketCORSRequest{Rules: rules})
}

func (r *BucketCORSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BucketCORSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	written, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error creating bucket CORS configuration", err.Error())
		return
	}

	r.apply(&plan, written)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketCORSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BucketCORSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rules, err := r.client.ListBucketCORSRules(ctx, state.BucketID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading bucket CORS configuration", err.Error())
		return
	}
	// An empty list is a bucket with no rules, which is what this resource looks
	// like once someone clears it out of band — the configuration is gone, not the
	// bucket, so the resource is removed rather than reported as an empty one.
	if len(rules) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	r.apply(&state, rules)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *BucketCORSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan BucketCORSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	written, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating bucket CORS configuration", err.Error())
		return
	}

	r.apply(&plan, written)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketCORSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BucketCORSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.ClearBucketCORSRules(ctx, state.BucketID.ValueString()); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting bucket CORS configuration", err.Error())
	}
}

// ImportState takes the bucket id, which is also this resource's id — a bucket
// has exactly one CORS configuration, so there is nothing else to name it by.
func (r *BucketCORSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("bucket_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// apply maps the API's rules onto the model.
func (r *BucketCORSResource) apply(m *BucketCORSResourceModel, rules []*client.BucketCORSRule) {
	m.ID = types.StringValue(m.BucketID.ValueString())
	out := make([]BucketCORSRuleTF, 0, len(rules))
	for _, rule := range rules {
		tf := BucketCORSRuleTF{
			AllowedOrigins: stringList(rule.AllowedOrigins),
			AllowedMethods: stringList(rule.AllowedMethods),
			AllowedHeaders: optionalStringList(rule.AllowedHeaders),
			ExposeHeaders:  optionalStringList(rule.ExposeHeaders),
		}
		if rule.MaxAgeSeconds > 0 {
			tf.MaxAgeSeconds = types.Int64Value(int64(rule.MaxAgeSeconds))
		} else {
			tf.MaxAgeSeconds = types.Int64Null()
		}
		out = append(out, tf)
	}
	m.Rules = out
}

// stringValues unwraps a list of framework strings.
func stringValues(in []types.String) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.ValueString())
	}
	return out
}

// stringList wraps plain strings as framework values.
func stringList(in []string) []types.String {
	out := make([]types.String, 0, len(in))
	for _, s := range in {
		out = append(out, types.StringValue(s))
	}
	return out
}

// optionalStringList is stringList for an attribute the config may omit: the API
// answers an omitted list as an empty one, and writing that back as `[]` would
// plan a permanent diff against a configuration that never mentioned it.
func optionalStringList(in []string) []types.String {
	if len(in) == 0 {
		return nil
	}
	return stringList(in)
}

package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &BucketDomainResource{}
	_ resource.ResourceWithConfigure = &BucketDomainResource{}
)

// NewBucketDomainResource returns a new bucket-domain resource.
func NewBucketDomainResource() resource.Resource {
	return &BucketDomainResource{}
}

// BucketDomainResource attaches a custom domain to a website bucket (#342).
type BucketDomainResource struct {
	client *client.Client
}

// BucketDomainResourceModel describes the resource data model.
type BucketDomainResourceModel struct {
	ID        types.String `tfsdk:"id"`
	BucketID  types.String `tfsdk:"bucket_id"`
	Domain    types.String `tfsdk:"domain"`
	Status    types.String `tfsdk:"status"`
	TLSStatus types.String `tfsdk:"tls_status"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (r *BucketDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bucket_domain"
}

func (r *BucketDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a custom domain attached to a website-enabled bucket. The domain starts " +
			"in pending_verification: add the TXT ownership record and point the domain at the platform " +
			"(CNAME for subdomains, A for an apex), then the platform serves it and TLS issues automatically. " +
			"The bucket must have its website enabled.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the domain.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bucket_id": schema.StringAttribute{
				Description: "The bucket ID to attach this domain to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain": schema.StringAttribute{
				Description: "The custom domain name (e.g. www.example.com).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "The current status of the domain (pending_verification, issuing, active, failed).",
				Computed:    true,
			},
			"tls_status": schema.StringAttribute{
				Description: "The TLS certificate status for the domain.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "The time the domain was created.",
				Computed:    true,
			},
		},
	}
}

func (r *BucketDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BucketDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BucketDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.AddBucketDomain(ctx, plan.BucketID.ValueString(), plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating bucket domain", err.Error())
		return
	}

	mapBucketDomainToState(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BucketDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No single-domain GET, so list the bucket's domains and find ours. Listing
	// also lazily reconciles verification server-side.
	domains, err := r.client.ListBucketDomains(ctx, state.BucketID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading bucket domains", err.Error())
		return
	}

	var found *client.Domain
	for _, d := range domains {
		if d.Domain == state.Domain.ValueString() {
			found = d
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	mapBucketDomainToState(found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *BucketDomainResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All fields are immutable (RequiresReplace), so Update should never be called.
	resp.Diagnostics.AddError(
		"Update not supported",
		"Bucket domain resources are immutable. Changes require replacement.",
	)
}

func (r *BucketDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BucketDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RemoveBucketDomain(ctx, state.BucketID.ValueString(), state.Domain.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting bucket domain", err.Error())
	}
}

// mapBucketDomainToState maps an API Domain response to the Terraform state model.
func mapBucketDomainToState(d *client.Domain, state *BucketDomainResourceModel) {
	state.ID = types.StringValue(d.ID)
	state.BucketID = types.StringValue(d.BucketID)
	state.Domain = types.StringValue(d.Domain)
	state.Status = types.StringValue(d.Status)
	state.TLSStatus = types.StringValue(d.TLSStatus)
	state.CreatedAt = types.StringValue(d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

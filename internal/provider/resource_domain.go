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
	_ resource.Resource              = &DomainResource{}
	_ resource.ResourceWithConfigure = &DomainResource{}
)

// NewDomainResource returns a new domain resource.
func NewDomainResource() resource.Resource {
	return &DomainResource{}
}

// DomainResource defines the resource implementation.
type DomainResource struct {
	client *client.Client
}

// DomainResourceModel describes the resource data model.
type DomainResourceModel struct {
	ID        types.String `tfsdk:"id"`
	AppID     types.String `tfsdk:"app_id"`
	Domain    types.String `tfsdk:"domain"`
	Status    types.String `tfsdk:"status"`
	TLSStatus types.String `tfsdk:"tls_status"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (r *DomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (r *DomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a custom domain attached to a Fogpipe application.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the domain.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"app_id": schema.StringAttribute{
				Description: "The application ID to attach this domain to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain": schema.StringAttribute{
				Description: "The custom domain name (e.g. app.example.com).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Description: "The current status of the domain.",
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

func (r *DomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.AddDomain(ctx, plan.AppID.ValueString(), plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating domain", err.Error())
		return
	}

	mapDomainToState(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No GetDomain by ID in the client, so list all domains and find ours.
	domains, err := r.client.ListDomains(ctx, state.AppID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading domains", err.Error())
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
		// Domain no longer exists.
		resp.State.RemoveResource(ctx)
		return
	}

	mapDomainToState(found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DomainResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All fields are immutable (RequiresReplace), so Update should never be called.
	resp.Diagnostics.AddError(
		"Update not supported",
		"Domain resources are immutable. Changes require replacement.",
	)
}

func (r *DomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RemoveDomain(ctx, state.AppID.ValueString(), state.Domain.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting domain", err.Error())
	}
}

// mapDomainToState maps an API Domain response to the Terraform state model.
func mapDomainToState(d *client.Domain, state *DomainResourceModel) {
	state.ID = types.StringValue(d.ID)
	state.AppID = types.StringValue(d.AppID)
	state.Domain = types.StringValue(d.Domain)
	state.Status = types.StringValue(d.Status)
	state.TLSStatus = types.StringValue(d.TLSStatus)
	state.CreatedAt = types.StringValue(d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
}

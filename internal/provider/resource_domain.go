package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	Mode      types.String `tfsdk:"mode"`
	Routes    types.List   `tfsdk:"routes"`
	Status    types.String `tfsdk:"status"`
	TLSStatus types.String `tfsdk:"tls_status"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// DomainRouteModel describes one path->app route in Terraform state.
type DomainRouteModel struct {
	Path    types.String `tfsdk:"path"`
	AppID   types.String `tfsdk:"app_id"`
	AppName types.String `tfsdk:"app_name"`
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
			"mode": schema.StringAttribute{
				Description: "Attachment mode: \"verified\" (default — TXT ownership + DNS pointing + " +
					"HTTP-01 cert), \"edge\" (any Host accepted, no verification/cert — TLS terminated " +
					"upstream), \"on_demand\" (DNS pointing only, HTTP-01 cert on attach — meant for an " +
					"app's own backend attaching its end users' domains), or \"wildcard\" (a leading " +
					"\"*.\" label, DNS-01 cert, requires a DNS-01 issuer configured on the platform). The " +
					"API has no update path for this field, so changing it replaces the domain.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"routes": schema.ListNestedAttribute{
				Description: "Path prefixes of this hostname served by a DIFFERENT app, so one origin can " +
					"be split across several apps (a frontend and an API deploy independently without " +
					"CORS or cross-site cookies). The app named by app_id above stays the catch-all: it " +
					"serves \"/\" and every path no route claims. The longest matching prefix wins, and " +
					"the request path is NOT rewritten — an app at '/api/' receives '/api/orders'. One " +
					"certificate covers the host however many apps sit behind it. Backends must be " +
					"always-on apps in the same project as the domain.",
				Optional: true,
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{
							Description: "Path prefix served by this backend, e.g. '/api/'. Must start with '/'; '/' alone is rejected (the domain's own app already serves it).",
							Required:    true,
						},
						"app_id": schema.StringAttribute{
							Description: "The app serving this prefix. Must be always-on and in the domain's project, and cannot be the domain's own app.",
							Required:    true,
						},
						"app_name": schema.StringAttribute{
							Description: "The backend app's name, resolved by the API for display.",
							Computed:    true,
						},
					},
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

	d, err := r.client.AddDomain(ctx, plan.AppID.ValueString(), plan.Domain.ValueString(), plan.Mode.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating domain", err.Error())
		return
	}

	// The create endpoint takes no route table, so a configured fan-out is a
	// second call. Applying it here rather than on the next apply matters: the
	// domain is routed as soon as its ownership checks pass, and until the table
	// lands every prefix is served by the catch-all app.
	routes := domainRoutesFromModel(ctx, plan.Routes, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(routes) > 0 {
		routed, err := r.client.SetDomainRoutes(ctx, plan.AppID.ValueString(), plan.Domain.ValueString(), routes)
		if err != nil {
			resp.Diagnostics.AddError("Error setting domain routes", err.Error())
			return
		}
		d = routed
	}

	mapDomainToState(d, &plan, &resp.Diagnostics)
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

	mapDomainToState(found, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update handles the one mutable field: the path->app route table. Every other
// attribute is RequiresReplace, so a change to any of them never reaches here.
func (r *DomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Replace-in-full, so an emptied list has to reach the API as an empty list
	// and not be skipped — that is how a fan-out is cleared.
	routes := domainRoutesFromModel(ctx, plan.Routes, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.SetDomainRoutes(ctx, plan.AppID.ValueString(), plan.Domain.ValueString(), routes)
	if err != nil {
		resp.Diagnostics.AddError("Error updating domain routes", err.Error())
		return
	}

	mapDomainToState(d, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
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
func mapDomainToState(d *client.Domain, state *DomainResourceModel, diags *diag.Diagnostics) {
	state.ID = types.StringValue(d.ID)
	state.AppID = types.StringValue(d.AppID)
	state.Domain = types.StringValue(d.Domain)
	state.Mode = types.StringValue(d.Mode)
	state.Status = types.StringValue(d.Status)
	state.TLSStatus = types.StringValue(d.TLSStatus)
	state.CreatedAt = types.StringValue(d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	setDomainRoutesOnModel(state, d.Routes, diags)
}

// domainRouteAttrTypes returns the attribute types for the domain route object.
func domainRouteAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"path":     types.StringType,
		"app_id":   types.StringType,
		"app_name": types.StringType,
	}
}

// setDomainRoutesOnModel converts the API's route table to the Terraform model.
// Routes are echoed by the API, so they are read back rather than preserved from
// the plan — a fan-out changed outside Terraform shows as drift.
func setDomainRoutesOnModel(model *DomainResourceModel, routes []client.DomainRoute, diags *diag.Diagnostics) {
	if len(routes) == 0 {
		model.Routes = types.ListNull(types.ObjectType{AttrTypes: domainRouteAttrTypes()})
		return
	}
	elems := make([]attr.Value, len(routes))
	for i, rt := range routes {
		obj, d := types.ObjectValue(domainRouteAttrTypes(), map[string]attr.Value{
			"path":     types.StringValue(rt.Path),
			"app_id":   types.StringValue(rt.AppID),
			"app_name": types.StringValue(rt.AppName),
		})
		diags.Append(d...)
		elems[i] = obj
	}
	list, d := types.ListValue(types.ObjectType{AttrTypes: domainRouteAttrTypes()}, elems)
	diags.Append(d...)
	model.Routes = list
}

// domainRoutesFromModel converts the configured route list into client routes.
// An unset list yields nil (no fan-out); an empty one yields an empty slice,
// which the endpoint reads as "clear the table".
func domainRoutesFromModel(ctx context.Context, l types.List, diags *diag.Diagnostics) []client.DomainRoute {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var models []DomainRouteModel
	diags.Append(l.ElementsAs(ctx, &models, false)...)
	routes := make([]client.DomainRoute, 0, len(models))
	for _, m := range models {
		routes = append(routes, client.DomainRoute{Path: m.Path.ValueString(), AppID: m.AppID.ValueString()})
	}
	return routes
}

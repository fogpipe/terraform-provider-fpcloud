package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &OrgDataSource{}

// OrgDataSource reads the organization a configuration builds inside.
//
// An org is read and never written here: it is granted by an operator together
// with its first owner (ADR-093), because it is the unit the platform bounds
// and invoices — a tenant able to declare one in Terraform could raise its own
// ceiling by declaring a second. Its ceiling and FKE entitlement are exposed as
// facts to read, not fields to set.
type OrgDataSource struct {
	client *client.Client
}

// OrgDataSourceModel describes the data source data model.
type OrgDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	FKEEnabled types.Bool   `tfsdk:"fke_enabled"`
	MaxCPU     types.String `tfsdk:"max_cpu"`
	MaxMemory  types.String `tfsdk:"max_memory"`
	MaxPods    types.Int64  `tfsdk:"max_pods"`
	MaxStorage types.String `tfsdk:"max_storage"`
	// The axes bounded in the control plane rather than by a ResourceQuota
	// (ADR-128): what the organization's bucket quotas may sum to, and what its
	// projects may hold in the registry.
	MaxObjectStorage   types.String `tfsdk:"max_object_storage"`
	MaxObjects         types.Int64  `tfsdk:"max_objects"`
	MaxRegistryStorage types.String `tfsdk:"max_registry_storage"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

// NewOrgDataSource returns a new OrgDataSource.
func NewOrgDataSource() datasource.DataSource {
	return &OrgDataSource{}
}

func (d *OrgDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org"
}

func (d *OrgDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Fogpipe organization. An organization is granted by an operator " +
			"together with its first owner, so there is no fpcloud_org resource to declare one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The organization's opaque id. Frozen, and what every namespace, " +
					"image path and hostname is derived from. Provide either id or name.",
				Optional: true,
				Computed: true,
			},
			"name": schema.StringAttribute{
				Description: "The organization's readable name. Mutable — nothing is derived from " +
					"it — so prefer id when pinning a configuration. Provide either id or name.",
				Optional: true,
				Computed: true,
			},
			"fke_enabled": schema.BoolAttribute{
				Description: "Whether the organization is entitled to FKE (tenant kubeconfig) access. " +
					"Operator-granted.",
				Computed: true,
			},
			"max_cpu": schema.StringAttribute{
				Description: "The organization's CPU ceiling, shared by every project it owns.",
				Computed:    true,
			},
			"max_memory": schema.StringAttribute{
				Description: "The organization's memory ceiling, shared by every project it owns.",
				Computed:    true,
			},
			"max_pods": schema.Int64Attribute{
				Description: "The organization's pod-count ceiling, shared by every project it owns.",
				Computed:    true,
			},
			"max_storage": schema.StringAttribute{
				Description: "The organization's persistent-volume ceiling, shared by every project it owns.",
				Computed:    true,
			},
			"max_object_storage": schema.StringAttribute{
				Description: "The organization's object-storage ceiling. Every bucket quota in the organization is a reservation against it, so a bucket the ceiling cannot hold is refused.",
				Computed:    true,
			},
			"max_objects": schema.Int64Attribute{
				Description: "The organization's object-count ceiling, summed the same way as max_object_storage.",
				Computed:    true,
			},
			"max_registry_storage": schema.StringAttribute{
				Description: "The organization's container-registry ceiling. Registry storage accrues on push rather than being declared, so it is enforced at the push against the last measurement.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the organization was created.",
				Computed:    true,
			},
		},
	}
}

func (d *OrgDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *OrgDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrgDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ref := data.ID.ValueString()
	if ref == "" {
		ref = data.Name.ValueString()
	}
	if ref == "" {
		resp.Diagnostics.AddError("Missing organization reference", "Provide either id or name.")
		return
	}

	org, err := d.client.GetOrg(ctx, ref)
	if err != nil {
		// The read endpoint resolves an id; a name is resolved against what the
		// caller can see, which is the only list a tenant is entitled to.
		found, ferr := d.findByName(ctx, ref)
		if ferr != nil {
			resp.Diagnostics.AddError("Error reading organization", err.Error())
			return
		}
		org = found
	}

	data.ID = types.StringValue(org.ShortID)
	data.Name = types.StringValue(org.DisplayName)
	data.FKEEnabled = types.BoolValue(org.FKEEnabled)
	data.MaxCPU = types.StringValue(org.MaxCPU)
	data.MaxMemory = types.StringValue(org.MaxMemory)
	data.MaxPods = types.Int64Value(int64(org.MaxPods))
	data.MaxStorage = types.StringValue(org.MaxStorage)
	data.MaxObjectStorage = types.StringValue(org.MaxObjectStorage)
	data.MaxObjects = types.Int64Value(org.MaxObjects)
	data.MaxRegistryStorage = types.StringValue(org.MaxRegistryStorage)
	data.CreatedAt = types.StringValue(org.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// findByName resolves an organization by its readable name, matched the way the
// control plane matches it — case-insensitively, since a name is free text.
func (d *OrgDataSource) findByName(ctx context.Context, name string) (*client.Organization, error) {
	orgs, err := d.client.ListOrgs(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range orgs {
		if strings.EqualFold(o.DisplayName, name) || o.ShortID == name {
			return o, nil
		}
	}
	return nil, fmt.Errorf("organization %q is %w", name, errNotAccessible)
}

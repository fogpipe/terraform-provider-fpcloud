package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SecretDataSource{}

// SecretDataSource reads an org secret bundle (Fogpipe Secrets Manager,
// ADR-028) so a configuration can wire real values into apps at apply time.
//
// The alternative it replaces was shelling out to the CLI and exporting the
// bundle into TF_VAR_*, which loses any multi-line value (a PEM, an APNs key)
// to line-based parsing and puts the plaintext in the shell's environment.
type SecretDataSource struct {
	client *client.Client
}

// SecretDataSourceModel describes the data source data model.
type SecretDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	OrgID   types.String `tfsdk:"org_id"`
	Name    types.String `tfsdk:"name"`
	Keys    types.List   `tfsdk:"keys"`
	Data    types.Map    `tfsdk:"data"`
	Targets types.List   `tfsdk:"targets"`
}

// NewSecretDataSource returns a new SecretDataSource.
func NewSecretDataSource() datasource.DataSource {
	return &SecretDataSource{}
}

func (d *SecretDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (d *SecretDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Read an organization secret bundle and its decrypted values. " +
			"Reading requires org write permission — revealing a bundle is a privileged, " +
			"audited operation. The values land in Terraform state in plaintext, so the " +
			"state backend must be treated as a secret store (see the remote-state guide).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The bundle's ID.",
				Computed:    true,
			},
			"org_id": schema.StringAttribute{
				Description: "Organization that owns the bundle.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Bundle name, e.g. \"argus-backend\".",
				Required:    true,
			},
			"keys": schema.ListAttribute{
				Description: "The bundle's key names, without values.",
				ElementType: types.StringType,
				Computed:    true,
			},
			"data": schema.MapAttribute{
				Description: "The bundle's decrypted key/value pairs.",
				ElementType: types.StringType,
				Computed:    true,
				Sensitive:   true,
			},
			"targets": schema.ListAttribute{
				Description: "Project IDs the bundle is mirrored into.",
				ElementType: types.StringType,
				Computed:    true,
			},
		},
	}
}

func (d *SecretDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SecretDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, err := d.client.GetOrgSecret(ctx, data.OrgID.ValueString(), data.Name.ValueString(), true)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Secret",
			fmt.Sprintf("Could not read secret %q in org %s: %s. Reading values requires org write permission.",
				data.Name.ValueString(), data.OrgID.ValueString(), err),
		)
		return
	}

	data.ID = types.StringValue(secret.ID)
	data.OrgID = types.StringValue(secret.OrgID)
	data.Name = types.StringValue(secret.Name)

	keys, diags := types.ListValueFrom(ctx, types.StringType, secret.Keys)
	resp.Diagnostics.Append(diags...)
	data.Keys = keys

	targets, diags := types.ListValueFrom(ctx, types.StringType, secret.Targets)
	resp.Diagnostics.Append(diags...)
	data.Targets = targets

	// A revealed bundle always carries Data; an empty map is still a valid answer
	// and must not become null, or `data.values["KEY"]` fails with a type error
	// rather than a missing-key one.
	values := secret.Data
	if values == nil {
		values = map[string]string{}
	}
	vals, diags := types.MapValueFrom(ctx, types.StringType, values)
	resp.Diagnostics.Append(diags...)
	data.Data = vals

	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

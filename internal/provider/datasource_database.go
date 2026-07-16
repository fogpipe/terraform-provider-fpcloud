package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &DatabaseDataSource{}

// DatabaseDataSource defines the data source implementation.
type DatabaseDataSource struct {
	client *client.Client
}

// DatabaseDataSourceModel describes the data source data model.
type DatabaseDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	ProjectID        types.String `tfsdk:"project_id"`
	Name             types.String `tfsdk:"name"`
	DisplayName      types.String `tfsdk:"display_name"`
	Engine           types.String `tfsdk:"engine"`
	Version          types.String `tfsdk:"version"`
	Plan             types.String `tfsdk:"plan"`
	Status           types.String `tfsdk:"status"`
	Host             types.String `tfsdk:"host"`
	Port             types.Int64  `tfsdk:"port"`
	Username         types.String `tfsdk:"username"`
	ConnectionString types.String `tfsdk:"connection_string"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

// NewDatabaseDataSource returns a new DatabaseDataSource.
func NewDatabaseDataSource() datasource.DataSource {
	return &DatabaseDataSource{}
}

func (d *DatabaseDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (d *DatabaseDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to read a Fogpipe managed database by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The database ID.",
				Required:    true,
			},
			"project_id": schema.StringAttribute{
				Description: "The ID of the project this database belongs to.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The database name.",
				Computed:    true,
			},
			"display_name": schema.StringAttribute{
				Description: "The human-readable display name.",
				Computed:    true,
			},
			"engine": schema.StringAttribute{
				Description: "The database engine (e.g. postgres).",
				Computed:    true,
			},
			"version": schema.StringAttribute{
				Description: "The database engine version.",
				Computed:    true,
			},
			"plan": schema.StringAttribute{
				Description: "The database plan (e.g. starter, standard, premium).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of the database.",
				Computed:    true,
			},
			"host": schema.StringAttribute{
				Description: "The database host address.",
				Computed:    true,
			},
			"port": schema.Int64Attribute{
				Description: "The database port.",
				Computed:    true,
			},
			"username": schema.StringAttribute{
				Description: "The database username.",
				Computed:    true,
			},
			"connection_string": schema.StringAttribute{
				Description: "The full connection string for the database.",
				Computed:    true,
				Sensitive:   true,
			},
			"created_at": schema.StringAttribute{
				Description: "The creation timestamp of the database.",
				Computed:    true,
			},
		},
	}
}

func (d *DatabaseDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DatabaseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DatabaseDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := d.client.GetDatabase(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Database",
			fmt.Sprintf("Could not read database ID %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	// Map response to model.
	data.ID = types.StringValue(db.ID)
	data.ProjectID = types.StringValue(db.ProjectID)
	data.Name = types.StringValue(db.Name)
	data.DisplayName = types.StringValue(db.DisplayName)
	data.Engine = types.StringValue(db.Engine)
	data.Version = types.StringValue(db.Version)
	data.Plan = types.StringValue(db.Plan)
	data.Status = types.StringValue(db.Status)
	data.Host = types.StringValue("")     // Host is not directly exposed; parsed from connection string if needed.
	data.Port = types.Int64Value(5432)    // Default PostgreSQL port.
	data.Username = types.StringValue("") // Username is not directly exposed in the API response.
	data.ConnectionString = types.StringValue(db.ConnectionString)
	data.CreatedAt = types.StringValue(db.CreatedAt.Format("2006-01-02T15:04:05Z"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

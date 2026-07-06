package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &AppDataSource{}

// AppDataSource defines the data source implementation.
type AppDataSource struct {
	client *client.Client
}

// AppDataSourceModel describes the data source data model.
type AppDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Name      types.String `tfsdk:"name"`
	Image     types.String `tfsdk:"image"`
	Port      types.Int64  `tfsdk:"port"`
	Status    types.String `tfsdk:"status"`
	URL       types.String `tfsdk:"url"`
	MinScale  types.Int64  `tfsdk:"min_scale"`
	MaxScale  types.Int64  `tfsdk:"max_scale"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// NewAppDataSource returns a new AppDataSource.
func NewAppDataSource() datasource.DataSource {
	return &AppDataSource{}
}

func (d *AppDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (d *AppDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to read a Fogpipe application by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The application ID.",
				Required:    true,
			},
			"project_id": schema.StringAttribute{
				Description: "The ID of the project this app belongs to.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The application name.",
				Computed:    true,
			},
			"image": schema.StringAttribute{
				Description: "The container image for the application.",
				Computed:    true,
			},
			"port": schema.Int64Attribute{
				Description: "The port the application listens on.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of the application.",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL where the application is accessible.",
				Computed:    true,
			},
			"min_scale": schema.Int64Attribute{
				Description: "The minimum number of instances.",
				Computed:    true,
			},
			"max_scale": schema.Int64Attribute{
				Description: "The maximum number of instances.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "The creation timestamp of the application.",
				Computed:    true,
			},
		},
	}
}

func (d *AppDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AppDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AppDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := d.client.GetApp(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read App",
			fmt.Sprintf("Could not read app ID %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	// Map response to model.
	data.ID = types.StringValue(app.ID)
	data.ProjectID = types.StringValue(app.ProjectID)
	data.Name = types.StringValue(app.Name)
	data.Image = types.StringValue(app.Image)
	data.Port = types.Int64Value(0) // Port is not exposed in the API response; default to 0.
	data.Status = types.StringValue(app.Status)
	data.URL = types.StringValue(app.URL)
	data.MinScale = types.Int64Value(int64(app.MinScale))
	data.MaxScale = types.Int64Value(int64(app.MaxScale))
	data.CreatedAt = types.StringValue(app.CreatedAt.Format("2006-01-02T15:04:05Z"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

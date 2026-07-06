package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ProjectDataSource{}

// ProjectDataSource defines the data source implementation.
type ProjectDataSource struct {
	client *client.Client
}

// ProjectDataSourceModel describes the data source data model.
type ProjectDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`
	Status      types.String `tfsdk:"status"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

// NewProjectDataSource returns a new ProjectDataSource.
func NewProjectDataSource() datasource.DataSource {
	return &ProjectDataSource{}
}

func (d *ProjectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *ProjectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to read a Fogpipe project by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The project ID. Provide either id or name.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The project name. Provide either id or name.",
				Optional:    true,
				Computed:    true,
			},
			"display_name": schema.StringAttribute{
				Description: "The display name of the project.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of the project.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "The creation timestamp of the project.",
				Computed:    true,
			},
		},
	}
}

func (d *ProjectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ProjectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ProjectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() && data.Name.IsNull() {
		resp.Diagnostics.AddError(
			"Missing Attribute",
			"Either id or name must be specified.",
		)
		return
	}

	var project *client.Project

	if !data.ID.IsNull() && data.ID.ValueString() != "" {
		// Lookup by ID.
		p, err := d.client.GetProject(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Read Project",
				fmt.Sprintf("Could not read project ID %s: %s", data.ID.ValueString(), err),
			)
			return
		}
		project = p
	} else {
		// Lookup by name: list all projects and filter.
		projects, err := d.client.ListProjects(ctx)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to List Projects",
				fmt.Sprintf("Could not list projects: %s", err),
			)
			return
		}

		targetName := data.Name.ValueString()
		for _, p := range projects {
			if p.Name == targetName {
				project = p
				break
			}
		}

		if project == nil {
			resp.Diagnostics.AddError(
				"Project Not Found",
				fmt.Sprintf("No project found with name %q.", targetName),
			)
			return
		}
	}

	// Map response to model.
	data.ID = types.StringValue(project.ID)
	data.Name = types.StringValue(project.Name)
	data.DisplayName = types.StringValue(project.Name) // display_name falls back to name
	data.Status = types.StringValue("active")
	data.CreatedAt = types.StringValue(project.CreatedAt.Format("2006-01-02T15:04:05Z"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

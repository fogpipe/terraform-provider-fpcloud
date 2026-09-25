package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &TemplateDataSource{}

// TemplateDataSource reads one entry of the curated app catalog (ADR-235).
// There is no resource that instantiates a template: OpenTofu's model is that
// the practitioner declares each resource, so a template here is a module
// written from this data source — the image, env, inputs and needs it reports
// are what a fpcloud_app, fpcloud_database and fpcloud_bucket take.
type TemplateDataSource struct {
	client *client.Client
}

// TemplateDataSourceModel describes the data source data model.
type TemplateDataSourceModel struct {
	Name            types.String `tfsdk:"name"`
	Summary         types.String `tfsdk:"summary"`
	Licence         types.String `tfsdk:"licence"`
	Homepage        types.String `tfsdk:"homepage"`
	UpstreamImage   types.String `tfsdk:"upstream_image"`
	UpstreamDigest  types.String `tfsdk:"upstream_digest"`
	Version         types.String `tfsdk:"version"`
	Image           types.String `tfsdk:"image"`
	Port            types.Int64  `tfsdk:"port"`
	HealthCheckPath types.String `tfsdk:"health_check_path"`
	CPU             types.String `tfsdk:"cpu"`
	Memory          types.String `tfsdk:"memory"`
	Command         types.List   `tfsdk:"command"`
	Env             types.Map    `tfsdk:"env"`
	Secrets         types.List   `tfsdk:"secrets"`
	Inputs          types.List   `tfsdk:"inputs"`
	DatabaseEngine  types.String `tfsdk:"database_engine"`
	DatabaseVersion types.String `tfsdk:"database_version"`
	DatabaseExts    types.List   `tfsdk:"database_extensions"`
	DatabaseCPU     types.String `tfsdk:"database_cpu"`
	DatabaseMemory  types.String `tfsdk:"database_memory"`
	DatabaseStorage types.String `tfsdk:"database_storage"`
	DatabaseInst    types.Int64  `tfsdk:"database_instances"`
	Reserved        types.Map    `tfsdk:"reserved"`
	Bucket          types.Bool   `tfsdk:"bucket"`
	BucketPublic    types.Bool   `tfsdk:"bucket_public_read"`
	RunAsUser       types.Int64  `tfsdk:"run_as_user"`
	StartupPeriod   types.Int64  `tfsdk:"startup_period_seconds"`
	StartupFailures types.Int64  `tfsdk:"startup_failure_threshold"`
	Notes           types.String `tfsdk:"notes"`
}

var templateInputAttrTypes = map[string]attr.Type{
	"key":      types.StringType,
	"prompt":   types.StringType,
	"type":     types.StringType,
	"required": types.BoolType,
	"default":  types.StringType,
}

// NewTemplateDataSource returns a new TemplateDataSource.
func NewTemplateDataSource() datasource.DataSource {
	return &TemplateDataSource{}
}

func (d *TemplateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template"
}

func (d *TemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to read one entry of the curated app catalog: the mirrored image, the environment it needs, the inputs it asks for and what it creates beside the app. Declare a fpcloud_database, a fpcloud_bucket and a fpcloud_app from it; the console and `fpcloud template deploy` do the same expansion in one step.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The catalog entry's name.",
				Required:    true,
			},
			"summary":  schema.StringAttribute{Description: "One line on what the app is.", Computed: true},
			"licence":  schema.StringAttribute{Description: "The upstream licence.", Computed: true},
			"homepage": schema.StringAttribute{Description: "The upstream project.", Computed: true},
			"upstream_image": schema.StringAttribute{
				Description: "The source image the platform's mirror was copied from.",
				Computed:    true,
			},
			"upstream_digest": schema.StringAttribute{
				Description: "The digest the source image was reviewed and mirrored at.",
				Computed:    true,
			},
			"version": schema.StringAttribute{
				Description: "The upstream version: the mirror's tag and the release name of every deploy made from it.",
				Computed:    true,
			},
			"image": schema.StringAttribute{
				Description: "The mirrored image reference an app is created with.",
				Computed:    true,
			},
			"port":              schema.Int64Attribute{Description: "The HTTP port the container listens on.", Computed: true},
			"health_check_path": schema.StringAttribute{Description: "The path that answers 200 once the app is up.", Computed: true},
			"cpu":               schema.StringAttribute{Description: "The recommended CPU limit.", Computed: true},
			"memory":            schema.StringAttribute{Description: "The recommended memory limit.", Computed: true},
			"command": schema.ListAttribute{
				Description: "The entrypoint override the entry needs, if any.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"env": schema.MapAttribute{
				Description: "The app's plain environment: literal values and $(NAME) references to what the platform injects for the bound database, bucket and the app's own URL.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"secrets": schema.ListAttribute{
				Description: "Keys the app needs as random secrets; generate one per key and pass them as the app's secrets.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"inputs": schema.ListNestedAttribute{
				Description: "What a person supplies: each lands in the app's environment (or secrets, for type secret) under key.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key":      schema.StringAttribute{Description: "The environment variable the value lands in.", Computed: true},
						"prompt":   schema.StringAttribute{Description: "What to ask for.", Computed: true},
						"type":     schema.StringAttribute{Description: "string, email or secret.", Computed: true},
						"required": schema.BoolAttribute{Description: "Whether the deploy is refused without it.", Computed: true},
						"default":  schema.StringAttribute{Description: "The value used when none is given.", Computed: true},
					},
				},
			},
			"database_engine": schema.StringAttribute{
				Description: "The managed database engine the entry needs; empty when it needs none.",
				Computed:    true,
			},
			"database_version": schema.StringAttribute{
				Description: "The database major the entry needs.",
				Computed:    true,
			},
			"database_extensions": schema.ListAttribute{
				Description: "Curated extensions the database needs installed.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"database_cpu": schema.StringAttribute{
				Description: "The CPU limit of each database instance the entry is created with; empty when it needs no database.",
				Computed:    true,
			},
			"database_memory": schema.StringAttribute{
				Description: "The memory limit of each database instance.",
				Computed:    true,
			},
			"database_storage": schema.StringAttribute{
				Description: "The volume of each database instance.",
				Computed:    true,
			},
			"database_instances": schema.Int64Attribute{
				Description: "How many instances every managed database runs; 0 when the entry needs no database.",
				Computed:    true,
			},
			"reserved": schema.MapAttribute{
				Description: "What a deployment of the entry holds while it runs, per hour, keyed by metered resource type (`compute.cpu`, `database.storage`) in that type's billing unit — cores or GiB. Multiplied by the org's rates (`fpcloud billing prices`) and 730 hours it is the monthly cost; bucket and backup storage are billed by use and absent.",
				ElementType: types.StringType,
				Computed:    true,
			},
			"bucket": schema.BoolAttribute{
				Description: "Whether the entry needs a bucket bound to the app.",
				Computed:    true,
			},
			"bucket_public_read": schema.BoolAttribute{
				Description: "Whether that bucket is created world-readable.",
				Computed:    true,
			},
			"run_as_user": schema.Int64Attribute{
				Description: "The uid the app is created to run as, pinned non-root, where the upstream image declares no user; null when the image's own user is what runs.",
				Computed:    true,
			},
			"startup_period_seconds": schema.Int64Attribute{
				Description: "How often the startup probe checks the health path during the first boot; null when the entry leaves startup to the shared health check.",
				Computed:    true,
			},
			"startup_failure_threshold": schema.Int64Attribute{
				Description: "How many startup probe misses the first boot is allowed before liveness takes over; null when the entry declares no startup probe.",
				Computed:    true,
			},
			"notes": schema.StringAttribute{
				Description: "What a person does after the deploy: the first sign-in, a setting the app only takes in its own UI.",
				Computed:    true,
			},
		},
	}
}

func (d *TemplateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *TemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data TemplateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	t, err := d.client.GetTemplate(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Template",
			fmt.Sprintf("Could not read catalog entry %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	data.Summary = types.StringValue(t.Summary)
	data.Licence = types.StringValue(t.Licence)
	data.Homepage = types.StringValue(t.Homepage)
	data.UpstreamImage = types.StringValue(t.Upstream.Image)
	data.UpstreamDigest = types.StringValue(t.Upstream.Digest)
	data.Version = types.StringValue(t.Version)
	data.Image = types.StringValue(t.Image)
	data.Port = types.Int64Value(int64(t.Port))
	data.HealthCheckPath = types.StringValue(t.HealthCheckPath)
	data.CPU = types.StringValue(t.Resources.CPU)
	data.Memory = types.StringValue(t.Resources.Memory)
	data.Notes = types.StringValue(t.Notes)

	diags := resp.Diagnostics
	data.Command, _ = types.ListValueFrom(ctx, types.StringType, orEmpty(t.Command))
	data.Env, _ = types.MapValueFrom(ctx, types.StringType, orEmptyMap(t.Env))
	data.Secrets, _ = types.ListValueFrom(ctx, types.StringType, orEmpty(t.Secrets))

	inputs := make([]attr.Value, 0, len(t.Inputs))
	for _, in := range t.Inputs {
		obj, d := types.ObjectValue(templateInputAttrTypes, map[string]attr.Value{
			"key":      types.StringValue(in.Key),
			"prompt":   types.StringValue(in.Prompt),
			"type":     types.StringValue(in.Type),
			"required": types.BoolValue(in.Required),
			"default":  types.StringValue(in.Default),
		})
		diags.Append(d...)
		inputs = append(inputs, obj)
	}
	list, d2 := types.ListValue(types.ObjectType{AttrTypes: templateInputAttrTypes}, inputs)
	diags.Append(d2...)
	data.Inputs = list

	if db := t.Needs.Database; db != nil {
		data.DatabaseEngine = types.StringValue(db.Engine)
		data.DatabaseVersion = types.StringValue(db.Version)
		data.DatabaseExts, _ = types.ListValueFrom(ctx, types.StringType, orEmpty(db.Extensions))
		data.DatabaseCPU = types.StringValue(db.CPU)
		data.DatabaseMemory = types.StringValue(db.Memory)
		data.DatabaseStorage = types.StringValue(db.Storage)
		data.DatabaseInst = types.Int64Value(int64(db.Instances))
	} else {
		data.DatabaseEngine = types.StringValue("")
		data.DatabaseVersion = types.StringValue("")
		data.DatabaseExts, _ = types.ListValueFrom(ctx, types.StringType, []string{})
		data.DatabaseCPU = types.StringValue("")
		data.DatabaseMemory = types.StringValue("")
		data.DatabaseStorage = types.StringValue("")
		data.DatabaseInst = types.Int64Value(0)
	}
	data.Reserved, _ = types.MapValueFrom(ctx, types.StringType, orEmptyMap(t.Reserved))
	if b := t.Needs.Bucket; b != nil {
		data.Bucket = types.BoolValue(true)
		data.BucketPublic = types.BoolValue(b.PublicRead)
	} else {
		data.Bucket = types.BoolValue(false)
		data.BucketPublic = types.BoolValue(false)
	}
	data.RunAsUser = types.Int64Null()
	if t.RunAsUser != nil {
		data.RunAsUser = types.Int64Value(*t.RunAsUser)
	}
	data.StartupPeriod, data.StartupFailures = types.Int64Null(), types.Int64Null()
	if s := t.Startup; s != nil {
		data.StartupPeriod = types.Int64Value(int64(s.PeriodSeconds))
		data.StartupFailures = types.Int64Value(int64(s.FailureThreshold))
	}

	resp.Diagnostics = diags
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

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
	_ resource.Resource              = &DatabaseBackupDestinationResource{}
	_ resource.ResourceWithConfigure = &DatabaseBackupDestinationResource{}
)

// NewDatabaseBackupDestinationResource returns a new database backup destination resource.
func NewDatabaseBackupDestinationResource() resource.Resource {
	return &DatabaseBackupDestinationResource{}
}

// DatabaseBackupDestinationResource defines the resource implementation.
type DatabaseBackupDestinationResource struct {
	client *client.Client
}

// DatabaseBackupDestinationResourceModel describes the resource data model.
type DatabaseBackupDestinationResourceModel struct {
	ID              types.String `tfsdk:"id"`
	DatabaseID      types.String `tfsdk:"database_id"`
	Provider        types.String `tfsdk:"provider_name"`
	Bucket          types.String `tfsdk:"bucket"`
	Region          types.String `tfsdk:"region"`
	Prefix          types.String `tfsdk:"prefix"`
	RoleARN         types.String `tfsdk:"role_arn"`
	WIFProvider     types.String `tfsdk:"wif_provider"`
	ServiceAccount  types.String `tfsdk:"service_account"`
	Audience        types.String `tfsdk:"audience"`
	Endpoint        types.String `tfsdk:"endpoint"`
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
	Schedule        types.String `tfsdk:"schedule"`
	LastRunAt       types.String `tfsdk:"last_run_at"`
	LastRunStatus   types.String `tfsdk:"last_run_status"`
}

func (r *DatabaseBackupDestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_backup_destination"
}

func (r *DatabaseBackupDestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a database's external (bring-your-own-bucket) backup destination — an " +
			"opt-in, per-database replication target in the customer's own bucket, in addition to the " +
			"platform-managed backup. Provider \"aws\"/\"gcp\" is keyless via OIDC federation (role_arn, " +
			"or wif_provider + service_account); provider \"s3\" is a static key (endpoint, " +
			"access_key_id, secret_access_key) for any S3-compatible store (Cloudflare R2, Backblaze B2, " +
			"Hetzner Object Storage, ...). One destination per database — a second resource for the same " +
			"database_id replaces it server-side.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Resource ID (same as database_id — one backup destination per database).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"database_id": schema.StringAttribute{
				Description: "ID of the database this backup destination belongs to. Changing it forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"provider_name": schema.StringAttribute{
				Description: "Backup target provider: \"aws\", \"gcp\", or \"s3\".",
				Required:    true,
			},
			"bucket": schema.StringAttribute{
				Description: "Name of the customer-owned bucket to back up to.",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "Bucket region.",
				Optional:    true,
				Computed:    true,
			},
			"prefix": schema.StringAttribute{
				Description: "Key prefix within the bucket to write backups under.",
				Optional:    true,
				Computed:    true,
			},
			"role_arn": schema.StringAttribute{
				Description: "AWS IAM role ARN to assume via web identity. Required for provider \"aws\".",
				Optional:    true,
				Computed:    true,
			},
			"wif_provider": schema.StringAttribute{
				Description: "GCP workload identity federation provider. Required for provider \"gcp\".",
				Optional:    true,
				Computed:    true,
			},
			"service_account": schema.StringAttribute{
				Description: "GCP service account to impersonate. Required for provider \"gcp\".",
				Optional:    true,
				Computed:    true,
			},
			"audience": schema.StringAttribute{
				Description: "OIDC audience the minted token carries. Defaults per-provider (sts.amazonaws.com for aws, wif_provider for gcp).",
				Optional:    true,
				Computed:    true,
			},
			"endpoint": schema.StringAttribute{
				Description: "S3 endpoint URL (e.g. https://<account>.r2.cloudflarestorage.com). Required for provider \"s3\".",
				Optional:    true,
				Computed:    true,
			},
			"access_key_id": schema.StringAttribute{
				Description: "S3 access key ID. Required for provider \"s3\".",
				Optional:    true,
				Computed:    true,
			},
			"secret_access_key": schema.StringAttribute{
				Description: "S3 secret access key. Required for provider \"s3\". Write-only — the API " +
					"never echoes it back; omitting it on update keeps the stored value.",
				Optional:  true,
				Sensitive: true,
			},
			"schedule": schema.StringAttribute{
				Description: "Cron schedule to sync backups on. Empty disables the scheduled sync (on-demand only).",
				Optional:    true,
				Computed:    true,
			},
			"last_run_at": schema.StringAttribute{
				Description: "Timestamp of the last sync run.",
				Computed:    true,
			},
			"last_run_status": schema.StringAttribute{
				Description: "Status of the last sync run.",
				Computed:    true,
			},
		},
	}
}

func (r *DatabaseBackupDestinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabaseBackupDestinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabaseBackupDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dest, err := r.client.SetBackupDestination(ctx, plan.DatabaseID.ValueString(), modelToBackupDestination(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating backup destination", err.Error())
		return
	}

	// The secret is write-only — the API never echoes it back — so keep the
	// configured value instead of trusting apply's (blank) result.
	configuredSecret := plan.SecretAccessKey
	r.apply(&plan, dest)
	plan.SecretAccessKey = configuredSecret
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseBackupDestinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabaseBackupDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dest, err := r.client.GetBackupDestination(ctx, state.DatabaseID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && (apiErr.StatusCode == 404) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading backup destination", err.Error())
		return
	}

	// The secret is write-only — the API never echoes it back — so preserve the
	// prior state value instead of letting apply blank it.
	priorSecret := state.SecretAccessKey
	r.apply(&state, dest)
	state.SecretAccessKey = priorSecret
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabaseBackupDestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabaseBackupDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dest, err := r.client.SetBackupDestination(ctx, plan.DatabaseID.ValueString(), modelToBackupDestination(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating backup destination", err.Error())
		return
	}

	configuredSecret := plan.SecretAccessKey
	r.apply(&plan, dest)
	plan.SecretAccessKey = configuredSecret
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseBackupDestinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabaseBackupDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteBackupDestination(ctx, state.DatabaseID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting backup destination", err.Error())
	}
}

// modelToBackupDestination builds the API request body from the plan. An empty
// secret_access_key is sent through as-is — the API keeps the stored secret when
// it receives a blank one on update.
func modelToBackupDestination(m *DatabaseBackupDestinationResourceModel) client.BackupDestination {
	return client.BackupDestination{
		Provider:        m.Provider.ValueString(),
		Bucket:          m.Bucket.ValueString(),
		Region:          m.Region.ValueString(),
		Prefix:          m.Prefix.ValueString(),
		RoleARN:         m.RoleARN.ValueString(),
		WIFProvider:     m.WIFProvider.ValueString(),
		ServiceAccount:  m.ServiceAccount.ValueString(),
		Audience:        m.Audience.ValueString(),
		Endpoint:        m.Endpoint.ValueString(),
		AccessKeyID:     m.AccessKeyID.ValueString(),
		SecretAccessKey: m.SecretAccessKey.ValueString(),
		Schedule:        m.Schedule.ValueString(),
	}
}

// apply maps an API BackupDestination response onto the model. secret_access_key
// is handled by the caller (it is never present in an API response).
func (r *DatabaseBackupDestinationResource) apply(m *DatabaseBackupDestinationResourceModel, dest *client.BackupDestination) {
	m.ID = m.DatabaseID
	m.Provider = types.StringValue(dest.Provider)
	m.Bucket = types.StringValue(dest.Bucket)
	m.Region = types.StringValue(dest.Region)
	m.Prefix = types.StringValue(dest.Prefix)
	m.RoleARN = types.StringValue(dest.RoleARN)
	m.WIFProvider = types.StringValue(dest.WIFProvider)
	m.ServiceAccount = types.StringValue(dest.ServiceAccount)
	m.Audience = types.StringValue(dest.Audience)
	m.Endpoint = types.StringValue(dest.Endpoint)
	m.AccessKeyID = types.StringValue(dest.AccessKeyID)
	m.Schedule = types.StringValue(dest.Schedule)
	m.LastRunAt = types.StringValue(dest.LastRunAt)
	m.LastRunStatus = types.StringValue(dest.LastRunStatus)
}

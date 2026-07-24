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

// NewDatabaseBackupDestinationResource returns a new backup-destination resource.
func NewDatabaseBackupDestinationResource() resource.Resource {
	return &DatabaseBackupDestinationResource{}
}

// DatabaseBackupDestinationResource manages a database's external (BYOB) backup
// destination — the customer's own bucket a database backs up directly to.
type DatabaseBackupDestinationResource struct {
	client *client.Client
}

// DatabaseBackupDestinationResourceModel describes the resource data model.
type DatabaseBackupDestinationResourceModel struct {
	ID              types.String `tfsdk:"id"`
	DatabaseID      types.String `tfsdk:"database_id"`
	Provider        types.String `tfsdk:"provider_type"`
	Bucket          types.String `tfsdk:"bucket"`
	Region          types.String `tfsdk:"region"`
	Prefix          types.String `tfsdk:"prefix"`
	FlatLayout      types.Bool   `tfsdk:"flat_layout"`
	RoleARN         types.String `tfsdk:"role_arn"`
	WIFProvider     types.String `tfsdk:"wif_provider"`
	ServiceAccount  types.String `tfsdk:"service_account"`
	Audience        types.String `tfsdk:"audience"`
	Endpoint        types.String `tfsdk:"endpoint"`
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
	Schedule        types.String `tfsdk:"schedule"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	LastRunAt       types.String `tfsdk:"last_run_at"`
	LastRunStatus   types.String `tfsdk:"last_run_status"`
}

func (r *DatabaseBackupDestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_backup_destination"
}

func (r *DatabaseBackupDestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a database's external (bring-your-own-bucket) backup destination. " +
			"Keyless via OIDC federation for AWS (role_arn) and GCP (wif_provider + service_account), " +
			"or a static key for any S3-compatible store (provider \"s3\": endpoint + access_key_id + secret_access_key) " +
			"— Cloudflare R2, Backblaze B2, Hetzner Object Storage, Garage.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The identifier of the backup destination (equals the database ID).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"database_id": schema.StringAttribute{
				Description: "The database this backup destination belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"provider_type": schema.StringAttribute{
				Description: "Backup provider: \"aws\" (keyless, role_arn), \"gcp\" (keyless, wif_provider + service_account), or \"s3\" (static key, endpoint + access_key_id + secret_access_key). Named provider_type because \"provider\" is a reserved Terraform argument.",
				Required:    true,
			},
			"bucket": schema.StringAttribute{
				Description: "The destination bucket name.",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "Bucket region. Required for aws; optional for s3 (e.g. \"auto\" for Cloudflare R2).",
				Optional:    true,
			},
			"prefix": schema.StringAttribute{
				Description: "Optional key prefix within the bucket.",
				Optional:    true,
			},
			"flat_layout": schema.BoolAttribute{
				Description: "Skip the <project>/<database> nesting fpcloud otherwise adds after prefix, so objects land at prefix/ (bucket root when prefix is also unset). Defaults to false (today's nested layout).",
				Optional:    true,
			},
			"role_arn": schema.StringAttribute{
				Description: "aws: the IAM role ARN assumed via web identity.",
				Optional:    true,
			},
			"wif_provider": schema.StringAttribute{
				Description: "gcp: the workload-identity provider resource.",
				Optional:    true,
			},
			"service_account": schema.StringAttribute{
				Description: "gcp: the target service-account email to impersonate.",
				Optional:    true,
			},
			"audience": schema.StringAttribute{
				Description: "Optional OIDC token audience (provider default otherwise).",
				Optional:    true,
			},
			"endpoint": schema.StringAttribute{
				Description: "s3: the custom endpoint URL (e.g. https://<acct>.r2.cloudflarestorage.com).",
				Optional:    true,
			},
			"access_key_id": schema.StringAttribute{
				Description: "s3: the static access key id.",
				Optional:    true,
			},
			"secret_access_key": schema.StringAttribute{
				Description: "s3: the static secret access key. Write-only — never returned by the API; the stored value is preserved across reads.",
				Optional:    true,
				Sensitive:   true,
			},
			"schedule": schema.StringAttribute{
				Description: "Cron schedule for automatic backups; empty = on-demand only.",
				Optional:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the backup destination is enabled.",
				Computed:    true,
			},
			"last_run_at": schema.StringAttribute{
				Description: "Timestamp of the last backup run.",
				Computed:    true,
			},
			"last_run_status": schema.StringAttribute{
				Description: "Status of the last backup run.",
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
	dest, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error creating backup destination", err.Error())
		return
	}
	mapBackupDestinationToState(dest, &plan)
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
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading backup destination", err.Error())
		return
	}
	// secret_access_key is write-only (never returned) — keep the state value.
	mapBackupDestinationToState(dest, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabaseBackupDestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabaseBackupDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	dest, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating backup destination", err.Error())
		return
	}
	mapBackupDestinationToState(dest, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseBackupDestinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabaseBackupDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteBackupDestination(ctx, state.DatabaseID.ValueString()); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting backup destination", err.Error())
	}
}

// set upserts the destination from the plan (shared by Create/Update). The
// secret_access_key from the plan is sent as-is (empty keeps the stored one); the
// API never returns it, so callers keep the plan value in state.
func (r *DatabaseBackupDestinationResource) set(ctx context.Context, plan *DatabaseBackupDestinationResourceModel) (*client.BackupDestination, error) {
	return r.client.SetBackupDestination(ctx, plan.DatabaseID.ValueString(), client.BackupDestination{
		Provider:        plan.Provider.ValueString(),
		Bucket:          plan.Bucket.ValueString(),
		Region:          plan.Region.ValueString(),
		Prefix:          plan.Prefix.ValueString(),
		FlatLayout:      plan.FlatLayout.ValueBool(),
		RoleARN:         plan.RoleARN.ValueString(),
		WIFProvider:     plan.WIFProvider.ValueString(),
		ServiceAccount:  plan.ServiceAccount.ValueString(),
		Audience:        plan.Audience.ValueString(),
		Endpoint:        plan.Endpoint.ValueString(),
		AccessKeyID:     plan.AccessKeyID.ValueString(),
		SecretAccessKey: plan.SecretAccessKey.ValueString(),
		Schedule:        plan.Schedule.ValueString(),
	})
}

// mapBackupDestinationToState maps an API response to Terraform state. The write-only
// secret_access_key is left untouched (the API never echoes it back), so whatever the
// plan/state already holds is preserved.
func mapBackupDestinationToState(dest *client.BackupDestination, state *DatabaseBackupDestinationResourceModel) {
	state.ID = state.DatabaseID
	state.Provider = types.StringValue(dest.Provider)
	state.Bucket = types.StringValue(dest.Bucket)
	state.Region = optionalString(dest.Region)
	state.Prefix = optionalString(dest.Prefix)
	state.FlatLayout = types.BoolValue(dest.FlatLayout)
	state.RoleARN = optionalString(dest.RoleARN)
	state.WIFProvider = optionalString(dest.WIFProvider)
	state.ServiceAccount = optionalString(dest.ServiceAccount)
	state.Audience = optionalString(dest.Audience)
	state.Endpoint = optionalString(dest.Endpoint)
	state.AccessKeyID = optionalString(dest.AccessKeyID)
	state.Schedule = optionalString(dest.Schedule)
	state.Enabled = types.BoolValue(dest.Enabled)
	state.LastRunAt = types.StringValue(dest.LastRunAt)
	state.LastRunStatus = types.StringValue(dest.LastRunStatus)
}

// optionalString preserves null for an empty optional attribute, so an omitted
// field doesn't show a perpetual "" ⇄ null diff.
func optionalString(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

package provider

import (
	"context"
	"fmt"
	"net/url"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &DatabaseResource{}
	_ resource.ResourceWithConfigure = &DatabaseResource{}
)

// NewDatabaseResource returns a new database resource.
func NewDatabaseResource() resource.Resource {
	return &DatabaseResource{}
}

// DatabaseResource defines the resource implementation.
type DatabaseResource struct {
	client *client.Client
}

// DatabaseBackupModel describes the backup configuration block.
type DatabaseBackupModel struct {
	Enabled   types.Bool   `tfsdk:"enabled"`
	Schedule  types.String `tfsdk:"schedule"`
	Retention types.String `tfsdk:"retention"`
}

// DatabaseResourceModel describes the resource data model.
type DatabaseResourceModel struct {
	ID               types.String         `tfsdk:"id"`
	ProjectID        types.String         `tfsdk:"project_id"`
	Name             types.String         `tfsdk:"name"`
	Engine           types.String         `tfsdk:"engine"`
	Version          types.String         `tfsdk:"version"`
	Plan             types.String         `tfsdk:"plan"`
	CPU              types.String         `tfsdk:"cpu"`
	Memory           types.String         `tfsdk:"memory"`
	Storage          types.String         `tfsdk:"storage"`
	Instances        types.Int64          `tfsdk:"instances"`
	Pooler           types.Bool           `tfsdk:"pooler"`
	Status           types.String         `tfsdk:"status"`
	Host             types.String         `tfsdk:"host"`
	Port             types.Int64          `tfsdk:"port"`
	Username         types.String         `tfsdk:"username"`
	Password         types.String         `tfsdk:"password"`
	ConnectionString types.String         `tfsdk:"connection_string"`
	CreatedAt        types.String         `tfsdk:"created_at"`
	Backup           *DatabaseBackupModel `tfsdk:"backup"`
}

func (r *DatabaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *DatabaseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe managed database.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the database.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "The project ID this database belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the database.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"engine": schema.StringAttribute{
				Description: "The database engine (e.g. postgres).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("postgres"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				Description: "The database engine major version (e.g. \"17\"). Mutable: raising it triggers an " +
					"in-place major-version upgrade (forward-only; the API rejects downgrades).",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("17"),
			},
			"plan": schema.StringAttribute{
				Description:        "Deprecated: use cpu/memory/storage/instances instead. Accepted but ignored by the API.",
				DeprecationMessage: "The `plan` attribute is ignored by the API; size the database with cpu, memory, storage, and instances instead.",
				Optional:           true,
				Computed:           true,
				Default:            stringdefault.StaticString("starter"),
			},
			"cpu": schema.StringAttribute{
				Description: "CPU request/limit (e.g. \"500m\", \"1\"). Mutable in place. The server applies its " +
					"default when unset; the API does not echo this back, so out-of-band changes are not detected.",
				Optional: true,
			},
			"memory": schema.StringAttribute{
				Description: "Memory request/limit (e.g. \"512Mi\", \"2Gi\"). Mutable in place. Server default applies " +
					"when unset; not echoed by the API, so out-of-band changes are not detected.",
				Optional: true,
			},
			"storage": schema.StringAttribute{
				Description: "Persistent volume size (e.g. \"10Gi\"). Mutable in place but grow-only — the API rejects " +
					"a shrink. Server default applies when unset; not echoed by the API, so out-of-band changes are not detected.",
				Optional: true,
			},
			"instances": schema.Int64Attribute{
				Description: "Number of Postgres instances (1 = single, >1 = HA replicas). Mutable in place. " +
					"Not settable at create time via this attribute — it is reconciled immediately after create.",
				Optional: true,
			},
			"pooler": schema.BoolAttribute{
				Description: "Whether a PgBouncer connection pooler is provisioned (injects DATABASE_POOL_URL). Mutable in place.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
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
			"password": schema.StringAttribute{
				Description: "The database password.",
				Computed:    true,
				Sensitive:   true,
			},
			"connection_string": schema.StringAttribute{
				Description: "The full connection string for the database.",
				Computed:    true,
				Sensitive:   true,
			},
			"created_at": schema.StringAttribute{
				Description: "The time the database was created.",
				Computed:    true,
			},
			"backup": schema.SingleNestedAttribute{
				Description: "Backup configuration for the database.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						Description: "Whether scheduled backups are enabled.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
					"schedule": schema.StringAttribute{
						Description: "Cron schedule for automated backups.",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("0 3 * * *"),
					},
					"retention": schema.StringAttribute{
						Description: "Backup retention period (e.g. 30d).",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("30d"),
					},
				},
			},
		},
	}
}

func (r *DatabaseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// cpu/memory/storage/pooler are accepted by CreateDatabase; the server applies
	// its defaults for any left unset. The legacy `plan` attribute is ignored.
	db, err := r.client.CreateDatabase(ctx, plan.ProjectID.ValueString(), client.CreateDatabaseRequest{
		Name:    plan.Name.ValueString(),
		Engine:  plan.Engine.ValueString(),
		Version: plan.Version.ValueString(),
		CPU:     plan.CPU.ValueString(),
		Memory:  plan.Memory.ValueString(),
		Storage: plan.Storage.ValueString(),
		Pooler:  plan.Pooler.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating database", err.Error())
		return
	}

	// instances has no create-time field on the API, so reconcile it in place
	// right after create when the user asked for more than the default single
	// instance.
	if !plan.Instances.IsNull() && !plan.Instances.IsUnknown() && plan.Instances.ValueInt64() > 0 {
		instances := plan.Instances.ValueInt64()
		updated, uerr := r.client.UpdateDatabase(ctx, db.ID, client.UpdateDatabaseRequest{Instances: &instances})
		if uerr != nil {
			resp.Diagnostics.AddError("Error setting database instances after creation", uerr.Error())
			return
		}
		db = updated
	}

	mapDatabaseToState(db, &plan)

	// Configure backup if specified.
	if plan.Backup != nil && plan.Backup.Enabled.ValueBool() {
		err := r.client.UpdateBackupConfig(ctx, db.ID, client.BackupConfig{
			Enabled:   plan.Backup.Enabled.ValueBool(),
			Schedule:  plan.Backup.Schedule.ValueString(),
			Retention: plan.Backup.Retention.ValueString(),
		})
		if err != nil {
			resp.Diagnostics.AddWarning("Error configuring backup", err.Error())
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, err := r.client.GetDatabase(ctx, state.ID.ValueString())
	if err != nil {
		// If the resource is not found, remove it from state.
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}

	mapDatabaseToState(db, &state)

	// Read backup config if the backup block is set in state.
	if state.Backup != nil {
		backupConfig, err := r.client.GetBackupConfig(ctx, state.ID.ValueString())
		if err == nil {
			state.Backup = &DatabaseBackupModel{
				Enabled:   types.BoolValue(backupConfig.Enabled),
				Schedule:  types.StringValue(backupConfig.Schedule),
				Retention: types.StringValue(backupConfig.Retention),
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DatabaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state DatabaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reconcile the mutable spec (cpu/memory/storage/version/instances/pooler) in
	// place via PATCH /databases/{id}. Only changed fields are sent.
	var updReq client.UpdateDatabaseRequest
	changed := false
	if !plan.CPU.Equal(state.CPU) {
		updReq.CPU = plan.CPU.ValueString()
		changed = true
	}
	if !plan.Memory.Equal(state.Memory) {
		updReq.Memory = plan.Memory.ValueString()
		changed = true
	}
	if !plan.Storage.Equal(state.Storage) {
		updReq.Storage = plan.Storage.ValueString()
		changed = true
	}
	if !plan.Version.Equal(state.Version) {
		updReq.Version = plan.Version.ValueString()
		changed = true
	}
	if !plan.Instances.Equal(state.Instances) && !plan.Instances.IsNull() {
		instances := plan.Instances.ValueInt64()
		updReq.Instances = &instances
		changed = true
	}
	if !plan.Pooler.Equal(state.Pooler) {
		pooler := plan.Pooler.ValueBool()
		updReq.Pooler = &pooler
		changed = true
	}

	if changed {
		db, err := r.client.UpdateDatabase(ctx, state.ID.ValueString(), updReq)
		if err != nil {
			resp.Diagnostics.AddError("Error updating database", err.Error())
			return
		}
		mapDatabaseToState(db, &plan)
	} else {
		// No spec change — carry the server-computed fields forward from prior
		// state so they don't go unknown.
		plan.ID = state.ID
		plan.Status = state.Status
		plan.Host = state.Host
		plan.Port = state.Port
		plan.Username = state.Username
		plan.Password = state.Password
		plan.ConnectionString = state.ConnectionString
		plan.CreatedAt = state.CreatedAt
	}

	// Reconcile backup config toward the desired state (idempotent).
	if plan.Backup != nil {
		if err := r.client.UpdateBackupConfig(ctx, state.ID.ValueString(), client.BackupConfig{
			Enabled:   plan.Backup.Enabled.ValueBool(),
			Schedule:  plan.Backup.Schedule.ValueString(),
			Retention: plan.Backup.Retention.ValueString(),
		}); err != nil {
			resp.Diagnostics.AddWarning("Error updating backup configuration", err.Error())
		}
	} else if state.Backup != nil {
		if err := r.client.UpdateBackupConfig(ctx, state.ID.ValueString(), client.BackupConfig{Enabled: false}); err != nil {
			resp.Diagnostics.AddWarning("Error disabling backup configuration", err.Error())
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDatabase(ctx, state.ID.ValueString())
	if err != nil {
		// Ignore 404 — already deleted.
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting database", err.Error())
	}
}

// mapDatabaseToState maps an API Database response to the Terraform state model.
func mapDatabaseToState(db *client.Database, state *DatabaseResourceModel) {
	state.ID = types.StringValue(db.ID)
	state.ProjectID = types.StringValue(db.ProjectID)
	state.Name = types.StringValue(db.Name)
	state.Engine = types.StringValue(db.Engine)
	state.Version = types.StringValue(db.Version)
	state.Plan = types.StringValue(db.Plan)
	state.Pooler = types.BoolValue(db.Pooler)
	state.Status = types.StringValue(db.Status)
	state.CreatedAt = types.StringValue(db.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	// Note: cpu/memory/storage/instances are intentionally NOT mapped here — the
	// API Database response does not echo them, so they are preserved from the
	// plan/state (write-only from Terraform's point of view).

	// Parse connection string to extract host/port/username/password.
	connStr := db.ConnectionString
	state.ConnectionString = types.StringValue(connStr)

	if connStr != "" {
		if parsed, err := url.Parse(connStr); err == nil {
			state.Host = types.StringValue(parsed.Hostname())
			if port := parsed.Port(); port != "" {
				// Convert string port to int64.
				var portNum int64
				for _, c := range port {
					portNum = portNum*10 + int64(c-'0')
				}
				state.Port = types.Int64Value(portNum)
			} else {
				state.Port = types.Int64Value(5432)
			}
			if parsed.User != nil {
				state.Username = types.StringValue(parsed.User.Username())
				if pw, ok := parsed.User.Password(); ok {
					state.Password = types.StringValue(pw)
				} else {
					state.Password = types.StringValue("")
				}
			} else {
				state.Username = types.StringValue("")
				state.Password = types.StringValue("")
			}
		} else {
			state.Host = types.StringValue("")
			state.Port = types.Int64Value(0)
			state.Username = types.StringValue("")
			state.Password = types.StringValue("")
		}
	} else {
		state.Host = types.StringValue("")
		state.Port = types.Int64Value(0)
		state.Username = types.StringValue("")
		state.Password = types.StringValue("")
	}
}

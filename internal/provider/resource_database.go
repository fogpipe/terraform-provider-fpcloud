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
				Description: "The database engine version.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("17"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"plan": schema.StringAttribute{
				Description: "The database plan (e.g. starter, standard, premium).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("starter"),
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

	// TODO: expose cpu/memory/storage as TF attributes; for now the server
	// applies its defaults. The legacy `plan` attribute is accepted but ignored.
	db, err := r.client.CreateDatabase(ctx, plan.ProjectID.ValueString(), client.CreateDatabaseRequest{
		Name:    plan.Name.ValueString(),
		Engine:  plan.Engine.ValueString(),
		Version: plan.Version.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating database", err.Error())
		return
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
	// The control-plane API now supports in-place reconcile (PATCH /databases/{id},
	// client.UpdateDatabase) for cpu/memory/storage/instances/version/pooler. The TF
	// schema does not yet expose those as mutable attributes — every current attribute
	// is RequiresReplace — so there is nothing for Update to reconcile. Exposing the
	// mutable resource attributes here is a follow-up (TASK-010).
	resp.Diagnostics.AddError(
		"Update not supported",
		"In-place updates aren't wired into the Terraform provider yet (the API supports them via PATCH /databases/{id}). Change an immutable field to trigger replacement, or use `fpcloud db update`.",
	)
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
	state.Status = types.StringValue(db.Status)
	state.CreatedAt = types.StringValue(db.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

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

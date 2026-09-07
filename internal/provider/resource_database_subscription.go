package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                = &DatabaseSubscriptionResource{}
	_ resource.ResourceWithConfigure   = &DatabaseSubscriptionResource{}
	_ resource.ResourceWithImportState = &DatabaseSubscriptionResource{}
)

// NewDatabaseSubscriptionResource returns a new subscription resource.
func NewDatabaseSubscriptionResource() resource.Resource {
	return &DatabaseSubscriptionResource{}
}

// DatabaseSubscriptionResource manages logical replication INTO a managed
// database from an external PostgreSQL, so moving onto fpcloud costs a
// connection switch rather than a full dump and restore.
type DatabaseSubscriptionResource struct {
	client *client.Client
}

// DatabaseSubscriptionResourceModel describes the resource data model.
type DatabaseSubscriptionResourceModel struct {
	ID                types.String `tfsdk:"id"`
	DatabaseID        types.String `tfsdk:"database_id"`
	Name              types.String `tfsdk:"name"`
	Publication       types.String `tfsdk:"publication"`
	PublicationDBName types.String `tfsdk:"publication_dbname"`
	Parameters        types.Map    `tfsdk:"parameters"`
	Source            types.Object `tfsdk:"source"`
	// Applied is the operand's answer to whether `CREATE SUBSCRIPTION` ran. It
	// stays true on a subscription that has since died, so it is beside Health
	// rather than part of it: they answer different questions and one of them
	// is not liveness.
	Applied   types.Bool   `tfsdk:"applied"`
	Message   types.String `tfsdk:"message"`
	Health    types.Object `tfsdk:"health"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// subscriptionSourceModel is the connection, plus the credential for it. The
// password is here and in no read: it is written into a Secret the platform
// names and returned by nothing.
type subscriptionSourceModel struct {
	Host     types.String `tfsdk:"host"`
	Port     types.Int32  `tfsdk:"port"`
	User     types.String `tfsdk:"user"`
	DBName   types.String `tfsdk:"dbname"`
	SSLMode  types.String `tfsdk:"sslmode"`
	Password types.String `tfsdk:"password"`
}

var subscriptionSourceTypes = map[string]attr.Type{
	"host":     types.StringType,
	"port":     types.Int32Type,
	"user":     types.StringType,
	"dbname":   types.StringType,
	"sslmode":  types.StringType,
	"password": types.StringType,
}

// subscriptionHealthModel is what the SUBSCRIBER reports, which is the only
// place the answer exists.
type subscriptionHealthModel struct {
	State         types.String `tfsdk:"state"`
	Reason        types.String `tfsdk:"reason"`
	Unreadable    types.String `tfsdk:"unreadable"`
	ApplyLagBytes types.Int64  `tfsdk:"apply_lag_bytes"`
	LastMessageAt types.String `tfsdk:"last_message_at"`
	ApplyErrors   types.Int64  `tfsdk:"apply_errors"`
	SyncErrors    types.Int64  `tfsdk:"sync_errors"`
}

var subscriptionHealthTypes = map[string]attr.Type{
	"state":           types.StringType,
	"reason":          types.StringType,
	"unreadable":      types.StringType,
	"apply_lag_bytes": types.Int64Type,
	"last_message_at": types.StringType,
	"apply_errors":    types.Int64Type,
	"sync_errors":     types.Int64Type,
}

func (r *DatabaseSubscriptionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_subscription"
}

func (r *DatabaseSubscriptionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Subscribes a managed database to a publication on an external PostgreSQL (logical replication), " +
			"so a migration onto fpcloud costs a connection switch instead of a full dump and restore. " +
			"Schema is NOT replicated: load the schema into the managed database first, then declare this — " +
			"a subscription against an empty database connects and moves nothing. " +
			"fpcloud opens egress from this database to the declared host and port for as long as the subscription exists, and to nothing else.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The identifier of the subscription (\"<database_id>/<name>\").",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"database_id": schema.StringAttribute{
				Description: "The managed database that subscribes.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The subscription's name in PostgreSQL. Lowercase letters, digits and underscores, starting with a letter.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"publication": schema.StringAttribute{
				Description: "The publication to subscribe to on the source.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"publication_dbname": schema.StringAttribute{
				Description: "The database holding the publication on the source, when it is not the one dbname names.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"parameters": schema.MapAttribute{
				Description: "CREATE SUBSCRIPTION WITH options. Allowlisted by the API: copy_data, create_slot, slot_name, binary, streaming, synchronous_commit, two_phase, disable_on_error, origin.",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"source": schema.SingleNestedAttribute{
				Description: "Where to read from. Every field forces replacement: PostgreSQL freezes a subscription's connection, and the egress allowance fpcloud opens was written for this host and port.",
				Required:    true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"host": schema.StringAttribute{
						Description: "The source host. Must resolve outside the cluster: a path, a cluster-internal name, loopback and the private ranges are refused.",
						Required:    true,
					},
					"port": schema.Int32Attribute{
						Description: "The source port. The egress allowance fpcloud opens is for this port, so a pooler on 6432 needs nothing else.",
						Optional:    true,
						Computed:    true,
						Default:     int32default.StaticInt32(5432),
					},
					"user": schema.StringAttribute{
						Description: "The role to connect to the source as.",
						Required:    true,
					},
					"dbname": schema.StringAttribute{
						Description: "The database to connect to on the source.",
						Required:    true,
					},
					"sslmode": schema.StringAttribute{
						Description: "libpq sslmode for the connection to the source. Defaults to \"require\".",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("require"),
					},
					"password": schema.StringAttribute{
						Description: "The source password. Write-only: it is written into a secret the platform owns and names, and is returned by no read.",
						Required:    true,
						Sensitive:   true,
					},
				},
			},
			"applied": schema.BoolAttribute{
				Description: "Whether CREATE SUBSCRIPTION ran. This stays true on a subscription that has since died — read health.state, not this, to know whether anything is flowing.",
				Computed:    true,
			},
			"message": schema.StringAttribute{
				Description: "The operand's reconciliation message, which is what says why the statement did not run.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "When the subscription was declared.",
				Computed:    true,
			},
			"health": schema.SingleNestedAttribute{
				Description: "What the SUBSCRIBER reports, read from pg_stat_subscription and pg_stat_subscription_stats — never from whether the platform managed to create the subscription.",
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"state": schema.StringAttribute{
						Description: "\"replicating\", \"down\", or \"unknown\" when the database could not be asked. Never empty.",
						Computed:    true,
					},
					"reason": schema.StringAttribute{
						Description: "Why the subscription is down: disabled, no apply worker, or not in the database at all.",
						Computed:    true,
					},
					"unreadable": schema.StringAttribute{
						Description: "Why nobody could ask. Set only with state \"unknown\", which is neither healthy nor broken.",
						Computed:    true,
					},
					"apply_lag_bytes": schema.Int64Attribute{
						Description: "WAL received from the source but not yet confirmed applied. -1 when the subscriber holds no position to compare, which is not zero lag.",
						Computed:    true,
					},
					"last_message_at": schema.StringAttribute{
						Description: "When the source last spoke to this subscriber.",
						Computed:    true,
					},
					"apply_errors": schema.Int64Attribute{
						Description: "Apply errors since the counters were last reset. -1 when unread.",
						Computed:    true,
					},
					"sync_errors": schema.Int64Attribute{
						Description: "Table-sync errors since the counters were last reset. -1 when unread.",
						Computed:    true,
					},
				},
			},
		},
	}
}

func (r *DatabaseSubscriptionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabaseSubscriptionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabaseSubscriptionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var source subscriptionSourceModel
	resp.Diagnostics.Append(plan.Source.As(ctx, &source, basetypes.ObjectAsOptions{})...)
	params := map[string]string{}
	if !plan.Parameters.IsNull() {
		resp.Diagnostics.Append(plan.Parameters.ElementsAs(ctx, &params, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	sub, err := r.client.CreateDatabaseSubscription(ctx, plan.DatabaseID.ValueString(), client.CreateDatabaseSubscriptionRequest{
		Name:              plan.Name.ValueString(),
		Publication:       plan.Publication.ValueString(),
		PublicationDBName: plan.PublicationDBName.ValueString(),
		Host:              source.Host.ValueString(),
		Port:              source.Port.ValueInt32(),
		User:              source.User.ValueString(),
		DBName:            source.DBName.ValueString(),
		SSLMode:           source.SSLMode.ValueString(),
		Password:          source.Password.ValueString(),
		Parameters:        params,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating database subscription", err.Error())
		return
	}
	resp.Diagnostics.Append(mapSubscriptionToState(ctx, sub, source.Password, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseSubscriptionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabaseSubscriptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var source subscriptionSourceModel
	resp.Diagnostics.Append(state.Source.As(ctx, &source, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}
	sub, err := r.client.GetDatabaseSubscription(ctx, state.DatabaseID.ValueString(), state.Name.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading database subscription", err.Error())
		return
	}
	// The password is write-only and never returned, so what state holds is
	// carried forward rather than read back.
	resp.Diagnostics.Append(mapSubscriptionToState(ctx, sub, source.Password, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update never runs: every configurable attribute forces replacement, because
// PostgreSQL freezes a subscription's name and database, the source connection
// is what the platform's egress allowance was written for, and the password
// cannot be edited without reading back a secret nothing reads back.
func (r *DatabaseSubscriptionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabaseSubscriptionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddError(
		"Database subscriptions cannot be updated in place",
		"Every argument of fpcloud_database_subscription forces replacement. This is a bug in the provider if you are seeing it.",
	)
}

func (r *DatabaseSubscriptionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabaseSubscriptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteDatabaseSubscription(ctx, state.DatabaseID.ValueString(), state.Name.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting database subscription", err.Error())
		return
	}
	resp.Diagnostics.AddWarning(
		"The replication slot on the source is yours",
		"Dropping a subscription drops the source's slot too when the source is reachable. If it was not, the slot remains and retains WAL on the source forever: check pg_replication_slots there.",
	)
}

// mapSubscriptionToState maps an API response to Terraform state. The password
// is carried in from the caller, because no read returns it.
func mapSubscriptionToState(ctx context.Context, sub *client.DatabaseSubscription, password types.String, state *DatabaseSubscriptionResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	state.ID = types.StringValue(sub.DatabaseID + "/" + sub.Name)
	state.DatabaseID = types.StringValue(sub.DatabaseID)
	state.Name = types.StringValue(sub.Name)
	state.Publication = types.StringValue(sub.Publication)
	state.PublicationDBName = optionalString(sub.PublicationDBName)
	state.Applied = types.BoolValue(sub.Applied)
	state.Message = types.StringValue(sub.Message)
	state.CreatedAt = types.StringValue(sub.CreatedAt.Format(time.RFC3339))

	source, d := types.ObjectValue(subscriptionSourceTypes, map[string]attr.Value{
		"host":     types.StringValue(sub.Source.Host),
		"port":     types.Int32Value(sub.Source.Port),
		"user":     types.StringValue(sub.Source.User),
		"dbname":   types.StringValue(sub.Source.DBName),
		"sslmode":  types.StringValue(sub.Source.SSLMode),
		"password": password,
	})
	diags.Append(d...)
	state.Source = source

	// -1 rather than 0 for every unread number: a lag nobody could read is not
	// a subscription that is caught up, and a null Computed attribute is not
	// something a config can compare against.
	health, d := types.ObjectValue(subscriptionHealthTypes, map[string]attr.Value{
		"state":           types.StringValue(sub.Health.State),
		"reason":          types.StringValue(sub.Health.Reason),
		"unreadable":      types.StringValue(sub.Health.Unreadable),
		"apply_lag_bytes": int64OrUnread(sub.Health.ApplyLagBytes),
		"last_message_at": subscriptionTimestamp(sub.Health.LastMessageAt),
		"apply_errors":    int64OrUnread(sub.Health.ApplyErrors),
		"sync_errors":     int64OrUnread(sub.Health.SyncErrors),
	})
	diags.Append(d...)
	state.Health = health
	return diags
}

func int64OrUnread(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Value(-1)
	}
	return types.Int64Value(*v)
}

func subscriptionTimestamp(v *time.Time) types.String {
	if v == nil {
		return types.StringValue("")
	}
	return types.StringValue(v.Format(time.RFC3339))
}

// ImportState takes "<database_id>/<name>". password imports as null and the
// first apply then replaces the subscription, which is stated rather than
// smoothed over: there is no way to import a credential nothing returns.
func (r *DatabaseSubscriptionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	dbID, name, found := strings.Cut(req.ID, "/")
	if !found || dbID == "" || name == "" {
		resp.Diagnostics.AddError(
			"Unexpected import identifier",
			fmt.Sprintf("Expected \"<database_id>/<name>\", got: %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("database_id"), dbID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

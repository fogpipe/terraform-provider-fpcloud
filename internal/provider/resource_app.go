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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                = &AppResource{}
	_ resource.ResourceWithImportState = &AppResource{}
)

// AppResource defines the resource implementation.
type AppResource struct {
	client *client.Client
}

// AppResourceModel describes the resource data model.
type AppResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	ProjectID           types.String `tfsdk:"project_id"`
	Name                types.String `tfsdk:"name"`
	DisplayName         types.String `tfsdk:"display_name"`
	URLSlug             types.String `tfsdk:"url_slug"`
	Database            types.String `tfsdk:"database"`
	Image               types.String `tfsdk:"image"`
	Command             types.List   `tfsdk:"command"`
	Args                types.List   `tfsdk:"args"`
	ReleaseCommand      types.List   `tfsdk:"release_command"`
	VolumeMounts        types.List   `tfsdk:"volume_mounts"`
	SecurityContext     types.Object `tfsdk:"security_context"`
	Port                types.Int64  `tfsdk:"port"`
	Ingress             types.String `tfsdk:"ingress"`
	Mode                types.String `tfsdk:"mode"`
	Type                types.String `tfsdk:"type"`
	Storage             types.String `tfsdk:"storage"`
	StoragePath         types.String `tfsdk:"storage_path"`
	ServiceAccount      types.String `tfsdk:"service_account"`
	Env                 types.Map    `tfsdk:"env"`
	Secret              types.Map    `tfsdk:"secret"`
	Replicas            types.Int64  `tfsdk:"replicas"`
	MinScale            types.Int64  `tfsdk:"min_scale"`
	MaxScale            types.Int64  `tfsdk:"max_scale"`
	CPULimit            types.String `tfsdk:"cpu_limit"`
	MemoryLimit         types.String `tfsdk:"memory_limit"`
	HealthCheckPath     types.String `tfsdk:"health_check_path"`
	HealthCheckTimeout  types.Int64  `tfsdk:"health_check_timeout"`
	HealthCheckInterval types.Int64  `tfsdk:"health_check_interval"`
	HealthCheckRetries  types.Int64  `tfsdk:"health_check_retries"`
	Probes              types.Object `tfsdk:"probes"`
	AdoptExisting       types.Bool   `tfsdk:"adopt_existing"`
	Routes              types.List   `tfsdk:"routes"`
	Traffic             types.List   `tfsdk:"traffic"`
	Status              types.String `tfsdk:"status"`
	URL                 types.String `tfsdk:"url"`
	CreatedAt           types.String `tfsdk:"created_at"`
	UpdatedAt           types.String `tfsdk:"updated_at"`
}

// RouteModel describes a per-path visibility carve-out in Terraform state.
type RouteModel struct {
	Path       types.String `tfsdk:"path"`
	Visibility types.String `tfsdk:"visibility"`
}

// TrafficTargetModel describes a traffic target in Terraform state.
type TrafficTargetModel struct {
	Revision types.String `tfsdk:"revision"`
	Percent  types.Int64  `tfsdk:"percent"`
	URL      types.String `tfsdk:"url"`
}

// VolumeMountModel describes a file/scratch volume mount in Terraform state.
type VolumeMountModel struct {
	Source    types.String `tfsdk:"source"`
	Name      types.String `tfsdk:"name"`
	MountPath types.String `tfsdk:"mount_path"`
	SubPath   types.String `tfsdk:"sub_path"`
}

// ProbeOverridesModel describes the per-probe overrides block in state. A null
// probe here means that probe keeps using the app's shared health_check_*
// settings, so overriding one never means restating the other two.
type ProbeOverridesModel struct {
	Liveness  types.Object `tfsdk:"liveness"`
	Readiness types.Object `tfsdk:"readiness"`
	Startup   types.Object `tfsdk:"startup"`
}

// ProbeSpecModel is one probe's path and timing in state. Every attribute is
// independently optional — a null one falls back to the matching health_check_*
// value rather than to a hardcoded default.
type ProbeSpecModel struct {
	Path                types.String `tfsdk:"path"`
	InitialDelaySeconds types.Int64  `tfsdk:"initial_delay_seconds"`
	PeriodSeconds       types.Int64  `tfsdk:"period_seconds"`
	TimeoutSeconds      types.Int64  `tfsdk:"timeout_seconds"`
	FailureThreshold    types.Int64  `tfsdk:"failure_threshold"`
	SuccessThreshold    types.Int64  `tfsdk:"success_threshold"`
}

// SecurityContextModel describes the pod/container hardening block in state.
type SecurityContextModel struct {
	RunAsUser              types.Int64 `tfsdk:"run_as_user"`
	RunAsGroup             types.Int64 `tfsdk:"run_as_group"`
	FSGroup                types.Int64 `tfsdk:"fs_group"`
	RunAsNonRoot           types.Bool  `tfsdk:"run_as_non_root"`
	ReadOnlyRootFilesystem types.Bool  `tfsdk:"read_only_root_filesystem"`
}

// NewAppResource returns a new app resource.
func NewAppResource() resource.Resource {
	return &AppResource{}
}

func (r *AppResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (r *AppResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Fogpipe application.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "App ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Description: "ID of the project this app belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "App name. Doubles as the frozen resource identity (namespace object names, " +
					"registry path), so changing it forces a new app.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable display name (mutable cosmetic label). Defaults to the name.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url_slug": schema.StringAttribute{
				Description: "Optional vanity host override (ADR-040): sets the app's public host to " +
					"'<url_slug>.app.<platform_domain>'. When empty, the host is derived from the app/" +
					"project/org names. Globally unique, a DNS-1123 label, always-on mode only. Set to an " +
					"empty string to clear it back to the derived host.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"database": schema.StringAttribute{
				Description: "Database (name or id) this app's unprefixed DATABASE_URL points at. " +
					"Leave unset when the project has a single database — that one is used. With several, " +
					"DATABASE_URL is omitted unless this names one; each database is always injected as " +
					"'<NAME>_DATABASE_URL' regardless. Set to an empty string to clear the binding.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"image": schema.StringAttribute{
				Description: "Container image to deploy.",
				Required:    true,
			},
			"command": schema.ListAttribute{
				Description: "Container entrypoint override (ENTRYPOINT). Read back from the API, so a " +
					"change made outside Terraform shows as drift.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"args": schema.ListAttribute{
				Description: "Container arguments (CMD/args). Read back from the API, so a change made " +
					"outside Terraform shows as drift.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"release_command": schema.ListAttribute{
				Description: "Command run once per deploy — from the exact image being deployed, with the " +
					"app's env/secrets — before the new version goes live; a failure aborts the deploy " +
					"(e.g. DB migrations). A single element containing spaces runs via 'sh -c'; use " +
					"multiple elements for exec form. Changed alongside the image, it is sent as part " +
					"of that deploy and gates it, so the apply that introduces a migration is the apply " +
					"that runs it. Read back from the API, so a change made outside Terraform shows as " +
					"drift.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"volume_mounts": schema.ListNestedAttribute{
				Description: "Mount a ConfigMap or Secret as read-only files, or an emptyDir as writable " +
					"scratch, at a container path. Create-only — the API has no update path, so any " +
					"change, including one made outside Terraform, forces the app to be replaced.",
				Optional: true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"source": schema.StringAttribute{
							Description: "Volume source: 'configmap', 'secret', or 'emptydir'.",
							Required:    true,
						},
						"name": schema.StringAttribute{
							Description: "ConfigMap or Secret name to mount (ignored for emptydir).",
							Optional:    true,
						},
						"mount_path": schema.StringAttribute{
							Description: "Container path to mount at.",
							Required:    true,
						},
						"sub_path": schema.StringAttribute{
							Description: "Mount a single key from the source instead of the whole directory.",
							Optional:    true,
						},
					},
				},
			},
			"security_context": schema.SingleNestedAttribute{
				Description: "Opt-in pod/container hardening. When set, the container is locked to the " +
					"PSS-restricted baseline (drop ALL capabilities, no privilege escalation, RuntimeDefault " +
					"seccomp) plus the run-as identity below. Updated in place: removing the block clears it, " +
					"which is how an app that once needed root returns to the platform default.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"run_as_user": schema.Int64Attribute{
						Description: "UID to run the container process as.",
						Optional:    true,
					},
					"run_as_group": schema.Int64Attribute{
						Description: "GID to run the container process as.",
						Optional:    true,
					},
					"fs_group": schema.Int64Attribute{
						Description: "Supplemental group applied to mounted volumes.",
						Optional:    true,
					},
					"run_as_non_root": schema.BoolAttribute{
						Description: "Require the container to run as a non-root user. Defaults to false.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
					"read_only_root_filesystem": schema.BoolAttribute{
						Description: "Mount the container root filesystem read-only. Defaults to false.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
				},
			},
			"port": schema.Int64Attribute{
				// No provider-side default: the API decides, because the default
				// depends on `type` — 8080 for a web app, none at all for a
				// worker, which has no port and refuses one. A static default
				// here would send 8080 for every unconfigured worker and turn a
				// valid config into an error.
				Description: "Container port. Defaults to 8080 on a web app; not valid on a worker.",
				Optional:    true,
				Computed:    true,
			},
			"ingress": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("internal"),
				Description: "Ingress setting: 'all' for public access, 'internal' for project-only (default)",
			},
			"mode": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("always-on"),
				Description: "Hosting mode: 'always-on' (plain Deployment, default) or 'serverless' (scale-to-zero Knative). Mutable in place — switches the running app over without recreating it.",
			},
			"type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("web"),
				Description: "Process type: 'web' (default) serves HTTP behind a Service, or 'worker' — a long-running process with no port, Service, ingress, URL or health checks. A worker is always-on only. Changing this replaces the app: it decides whether the app has an address at all.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"storage": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Persistent volume size (e.g. '50Gi'). Opt-in and always-on mode only. Grow-only — the volume can never shrink.",
			},
			"storage_path": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Mount path for the persistent volume. Defaults to '/data' when storage is set. Immutable — changing it replaces the app.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_account": schema.StringAttribute{
				Optional:    true,
				Description: "Service account email to attach as workload identity. The app will receive credentials to call the Fogpipe API as this service account.",
			},
			"env": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Environment variables (plaintext). Set as part of the create, so a release " +
					"command on a new app reads them on its first run.",
			},
			"secret": schema.MapAttribute{
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "Secret environment variables (encrypted at rest). Set as part of the create, " +
					"like env, so a release command that reads one is not gated before it arrives.",
			},
			"replicas": schema.Int64Attribute{
				Description: "Fixed replica count for always-on apps. Defaults to 1. Ignored for serverless apps, which scale via min_scale/max_scale.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
			},
			"min_scale": schema.Int64Attribute{
				// No static default: min_scale is mode-dependent (always-on and
				// serverless resolve it differently server-side), so a fixed
				// default would fight the API's computed value and produce a
				// "provider produced inconsistent result after apply". Left
				// Computed, the API value flows in when unset; UseStateForUnknown
				// keeps it stable across updates that don't touch scaling.
				Description: "Minimum number of instances. Server-computed when unset.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"max_scale": schema.Int64Attribute{
				Description: "Maximum number of instances. Server-computed when unset.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"cpu_limit": schema.StringAttribute{
				Description: "CPU limit (e.g. 500m). Defaults to 500m.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("500m"),
			},
			"memory_limit": schema.StringAttribute{
				Description: "Memory limit (e.g. 512Mi). Defaults to 512Mi.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("512Mi"),
			},
			"health_check_path": schema.StringAttribute{
				// Defaulted by the API for the same reason as `port`: a worker
				// has no port to probe and refuses a health path.
				Description: "HTTP path for health checks. Defaults to '/' on a web app; not valid on a worker. Set a custom path (e.g. '/healthz') to enable startup probes.",
				Optional:    true,
				Computed:    true,
			},
			"health_check_timeout": schema.Int64Attribute{
				Description: "Health check timeout in seconds. Defaults to 5.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(5),
			},
			"health_check_interval": schema.Int64Attribute{
				Description: "Health check interval in seconds. Defaults to 10.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(10),
			},
			"health_check_retries": schema.Int64Attribute{
				Description: "Health check failure threshold. Defaults to 3.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(3),
			},
			"probes": schema.SingleNestedAttribute{
				Description: "Per-probe overrides for liveness, readiness and startup. By default all " +
					"three run the app's health_check_* settings, so one request decides both whether " +
					"traffic reaches the app and whether the pod is restarted. Point liveness at a cheap " +
					"path that touches no downstream and readiness at the one that does, and a dependency " +
					"blip pulls the pod out of the load balancer without killing it. Any attribute left " +
					"unset keeps the matching health_check_* value, so overriding a path never means " +
					"restating the timing. Always-on apps only.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"liveness":  probeSchema("Restarts the pod when it fails. Kubernetes holds it off until the startup probe first succeeds."),
					"readiness": probeSchema("Decides whether traffic reaches the pod. Failing it only pulls the pod out of the Service until it recovers."),
					"startup":   probeSchema("Gates the other two until the app has booted. Raise failure_threshold for a slow start rather than delaying liveness."),
				},
			},
			"adopt_existing": schema.BoolAttribute{
				Description: "When true, if an app with this name already exists in the project, adopt it " +
					"into Terraform state on create instead of failing with a 409 conflict. Defaults to " +
					"false, so create never silently takes ownership of an app it did not create. Note: " +
					"adoption records the existing app in state but does not push the configured image/env/" +
					"secret — run a subsequent apply to reconcile them.",
				Optional: true,
			},
			"routes": schema.ListNestedAttribute{
				Description: "Per-path visibility carve-outs. A route marked internal is withheld from the " +
					"external ingress — on the app's own URL and on every custom domain attached to it — " +
					"while staying reachable at the app's in-cluster address, so a scheduled job calling it " +
					"keeps working. External requests are refused at the edge. Matching is by path prefix on " +
					"segment boundaries ('/internal/' covers '/internal/sync' but not '/internalx'). " +
					"Always-on apps with ingress = \"all\" only.",
				Optional: true,
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{
							Description: "Path prefix to carve out, e.g. '/internal/'. Must start with '/'; '/' alone is rejected (use ingress = \"internal\" to make every path cluster-only).",
							Optional:    true,
							Computed:    true,
						},
						"visibility": schema.StringAttribute{
							Description: "'internal' (not externally routable) or 'public'. Defaults to 'internal'.",
							Optional:    true,
							Computed:    true,
						},
					},
				},
			},
			"traffic": schema.ListNestedAttribute{
				Description: "Traffic routing configuration. Each block specifies a revision and its traffic percentage. Use '@latest' to route to the latest revision.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					// Optional+Computed rather than Required, because the whole
					// attribute is Optional+Computed and the API fills it in. On
					// a config that never writes a traffic block, Terraform's
					// proposed new state descends into the element the provider
					// read back and nulls every nested attribute the config does
					// not supply — a Required one has nowhere to take a value
					// from. That difference from prior state can never be
					// reconciled, and it marks every other config-null computed
					// attribute unknown, so the plan is non-empty forever
					// (fogpipe/cloud-workspace#226). Only a serverless app has
					// traffic at all, which is why the mode switch was where it
					// showed.
					Attributes: map[string]schema.Attribute{
						"revision": schema.StringAttribute{
							Description: "Revision name or '@latest' to route to the latest revision.",
							Optional:    true,
							Computed:    true,
						},
						"percent": schema.Int64Attribute{
							Description: "Traffic percentage (0-100). All percentages must sum to 100.",
							Optional:    true,
							Computed:    true,
						},
						"url": schema.StringAttribute{
							Description: "URL for this traffic target (computed by Knative).",
							Computed:    true,
						},
					},
				},
			},
			"status": schema.StringAttribute{
				Description: "Current status of the app.",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "URL where the app is accessible: the platform host while it is served, or the oldest active custom domain once one exists — an active domain replaces the platform host (ADR-130), so this changes when a domain activates or is removed.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the app was created.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the app was last updated.",
				Computed:    true,
			},
		},
	}
}

func (r *AppResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *AppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AppResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// env and secret ride the create request (ADR-112): an app created with a
	// release command is gated on it before it serves, so config written after
	// the create call arrives after the migration that reads it has already run.
	var envMap, secretMap map[string]string

	if !plan.Env.IsNull() && !plan.Env.IsUnknown() {
		resp.Diagnostics.Append(plan.Env.ElementsAs(ctx, &envMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if !plan.Secret.IsNull() && !plan.Secret.IsUnknown() {
		resp.Diagnostics.Append(plan.Secret.ElementsAs(ctx, &secretMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	command := stringListToSlice(ctx, plan.Command, &resp.Diagnostics)
	args := stringListToSlice(ctx, plan.Args, &resp.Diagnostics)
	releaseCommand := stringListToSlice(ctx, plan.ReleaseCommand, &resp.Diagnostics)
	volumeMounts := volumeMountsFromModel(ctx, plan.VolumeMounts, &resp.Diagnostics)
	securityContext := securityContextFromModel(ctx, plan.SecurityContext, &resp.Diagnostics)
	routes := routesFromModel(ctx, plan.Routes, &resp.Diagnostics)
	probes := probesFromModel(ctx, plan.Probes, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateAppRequest{
		Name:                plan.Name.ValueString(),
		DisplayName:         plan.DisplayName.ValueString(),
		URLSlug:             plan.URLSlug.ValueString(),
		Image:               plan.Image.ValueString(),
		Command:             command,
		Args:                args,
		ReleaseCommand:      releaseCommand,
		VolumeMounts:        volumeMounts,
		SecurityContext:     securityContext,
		Port:                int(plan.Port.ValueInt64()),
		Ingress:             plan.Ingress.ValueString(),
		Routes:              routes,
		Mode:                plan.Mode.ValueString(),
		Type:                plan.Type.ValueString(),
		Storage:             plan.Storage.ValueString(),
		StoragePath:         plan.StoragePath.ValueString(),
		HealthCheckPath:     plan.HealthCheckPath.ValueString(),
		HealthCheckTimeout:  int(plan.HealthCheckTimeout.ValueInt64()),
		HealthCheckInterval: int(plan.HealthCheckInterval.ValueInt64()),
		HealthCheckRetries:  int(plan.HealthCheckRetries.ValueInt64()),
		Probes:              probes,
		EnvVars:             envMap,
		Secrets:             secretMap,
	}
	if !plan.ServiceAccount.IsNull() && !plan.ServiceAccount.IsUnknown() {
		createReq.ServiceAccount = plan.ServiceAccount.ValueString()
	}
	// Replicas is an always-on setting; leave it unset for serverless apps.
	if plan.Mode.ValueString() != "serverless" {
		createReq.Replicas = int(plan.Replicas.ValueInt64())
	}

	app, err := r.client.CreateApp(ctx, plan.ProjectID.ValueString(), createReq)
	if err != nil {
		if isConflict(err) && plan.AdoptExisting.ValueBool() {
			existing, ferr := r.findAppByName(ctx, plan.ProjectID.ValueString(), plan.Name.ValueString())
			if ferr != nil {
				resp.Diagnostics.AddError(
					"Error adopting existing app",
					adoptErrorDetail("app", plan.Name.ValueString(), ferr),
				)
				return
			}
			// Record the existing app in state as-is. Image/env/secret/scaling are
			// not pushed here; a subsequent apply reconciles them against the config.
			r.setModelFromApp(&plan, existing, &resp.Diagnostics)
			if targets, terr := r.client.GetTraffic(ctx, existing.ID); terr == nil {
				r.setTrafficOnModel(ctx, &plan, targets, &resp.Diagnostics)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
		resp.Diagnostics.AddError("Error creating app", err.Error())
		return
	}

	// The database binding is a separate call: the create request has no field
	// for it (the API takes it on PATCH), and an unset value means "the project's
	// sole database", which is already the default.
	if !plan.Database.IsNull() && !plan.Database.IsUnknown() && plan.Database.ValueString() != "" {
		bound, err := r.client.SetAppDatabase(ctx, app.ID, plan.Database.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error setting app database binding", err.Error())
			return
		}
		app = bound
	}

	// An app created with a release command is gated on it before it serves, so
	// the apply waits for the same verdict a deploy does.
	if len(plan.ReleaseCommand.Elements()) > 0 {
		if err := awaitRelease(ctx, r.client, app.ID); err != nil {
			resp.Diagnostics.AddError("Error running release command", err.Error())
			return
		}
	}

	// Reconcile scaling after create. min/max scale are sent only when the user
	// set them (they have no static default now — see the schema), so an unset
	// value picks up the API's mode-appropriate default without a plan-vs-apply
	// drift.
	replicas := int32(plan.Replicas.ValueInt64())
	cpuLimit := plan.CPULimit.ValueString()
	memoryLimit := plan.MemoryLimit.ValueString()
	alwaysOn := plan.Mode.ValueString() != "serverless"

	scaleReq := client.ScaleRequest{
		CPULimit:    cpuLimit,
		MemoryLimit: memoryLimit,
	}
	sendScale := cpuLimit != "500m" || memoryLimit != "512Mi" || (alwaysOn && replicas != 1)
	if !plan.MinScale.IsNull() {
		m := int32(plan.MinScale.ValueInt64())
		scaleReq.MinScale = &m
		sendScale = true
	}
	if !plan.MaxScale.IsNull() {
		m := int32(plan.MaxScale.ValueInt64())
		scaleReq.MaxScale = &m
		sendScale = true
	}
	// Replicas is an always-on setting; sending it on a serverless app is a 400.
	if alwaysOn {
		scaleReq.Replicas = &replicas
	}
	if sendScale {
		scaled, err := r.client.ScaleApp(ctx, app.ID, scaleReq)
		if err != nil {
			resp.Diagnostics.AddError("Error scaling app after creation", err.Error())
			return
		}
		app = scaled
	}

	// Apply traffic configuration if specified.
	if !plan.Traffic.IsNull() && !plan.Traffic.IsUnknown() {
		var trafficModels []TrafficTargetModel
		resp.Diagnostics.Append(plan.Traffic.ElementsAs(ctx, &trafficModels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		if len(trafficModels) > 0 {
			targets := make([]client.TrafficTarget, len(trafficModels))
			for i, tm := range trafficModels {
				targets[i] = client.TrafficTarget{
					Revision: tm.Revision.ValueString(),
					Percent:  tm.Percent.ValueInt64(),
				}
			}

			result, err := r.client.SetTraffic(ctx, app.ID, targets)
			if err != nil {
				resp.Diagnostics.AddError("Error setting traffic", err.Error())
				return
			}
			r.setTrafficOnModel(ctx, &plan, result, &resp.Diagnostics)
		}
	}

	r.setModelFromApp(&plan, app, &resp.Diagnostics)
	if plan.Traffic.IsNull() || plan.Traffic.IsUnknown() {
		// Read current traffic from the API. On error (e.g. a freshly-created
		// serverless app with no ready revision yet) fall back to an empty set so
		// traffic resolves to a known value — a Computed attribute left unknown
		// after apply is an "invalid result object" error.
		targets, err := r.client.GetTraffic(ctx, app.ID)
		if err != nil {
			targets = nil
		}
		r.setTrafficOnModel(ctx, &plan, targets, &resp.Diagnostics)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AppResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := r.client.GetApp(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading app", err.Error())
		return
	}

	r.setModelFromApp(&state, app, &resp.Diagnostics)

	// Read current traffic.
	targets, err := r.client.GetTraffic(ctx, state.ID.ValueString())
	if err == nil {
		r.setTrafficOnModel(ctx, &state, targets, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// awaitRelease blocks until the app's newest deployment leaves the release gate
// (ADR-042), and fails the apply if the gate did. The gate runs in the
// background so a migration can outlive an HTTP timeout, which means an apply
// that did not wait would report success while the release command was still
// running — or had already failed and left the previous version serving.
func awaitRelease(ctx context.Context, c *client.Client, appID string) error {
	deadline := time.Now().Add(30 * time.Minute)
	for {
		deps, err := c.ListDeployments(ctx, appID)
		if err != nil {
			return err
		}
		if len(deps) > 0 {
			switch deps[0].Status {
			case "deploying", "releasing":
			case "failed":
				return fmt.Errorf("the release command failed; `fpcloud app deployments %s` has its output", appID)
			default:
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for the release command to finish; `fpcloud app deployments %s` has its state", appID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (r *AppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AppResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	appID := state.ID.ValueString()

	deployingImage := plan.Image.ValueString() != state.Image.ValueString()
	releaseCommandChanged := !plan.ReleaseCommand.Equal(state.ReleaseCommand)

	// Update the container command/args/release command if any changed. A
	// non-nil pointer (including an empty slice) replaces the value; an empty
	// slice clears the override back to the image defaults (or drops the
	// release phase).
	//
	// A release command changing alongside the image is not sent here at all —
	// it rides the deploy below, which is the request that runs it (ADR-110).
	// Configured separately, the apply that introduces a release command is the
	// one deploy that does not run it, and reports success having rolled out
	// new code past the migration that was added to gate it.
	if !plan.Command.Equal(state.Command) || !plan.Args.Equal(state.Args) || (releaseCommandChanged && !deployingImage) {
		command := stringListToSlice(ctx, plan.Command, &resp.Diagnostics)
		args := stringListToSlice(ctx, plan.Args, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		var releaseCommand *[]string
		if releaseCommandChanged && !deployingImage {
			v := stringListToSlice(ctx, plan.ReleaseCommand, &resp.Diagnostics)
			if resp.Diagnostics.HasError() {
				return
			}
			releaseCommand = &v
		}
		_, err := r.client.UpdateAppCommand(ctx, appID, &command, &args, releaseCommand)
		if err != nil {
			resp.Diagnostics.AddError("Error updating app command", err.Error())
			return
		}
	}

	// Deploy new image if it changed.
	if deployingImage {
		deployReq := client.DeployRequest{Image: plan.Image.ValueString()}
		if releaseCommandChanged {
			v := stringListToSlice(ctx, plan.ReleaseCommand, &resp.Diagnostics)
			if resp.Diagnostics.HasError() {
				return
			}
			deployReq.ReleaseCommand = &v
		}
		_, err := r.client.DeployApp(ctx, appID, deployReq)
		if err != nil {
			resp.Diagnostics.AddError("Error deploying app", err.Error())
			return
		}
		if len(plan.ReleaseCommand.Elements()) > 0 {
			if err := awaitRelease(ctx, r.client, appID); err != nil {
				resp.Diagnostics.AddError("Error running release command", err.Error())
				return
			}
		}
	}

	// Update the cosmetic display name if it changed.
	if plan.DisplayName.ValueString() != state.DisplayName.ValueString() {
		_, err := r.client.UpdateAppDisplayName(ctx, appID, plan.DisplayName.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating app display name", err.Error())
			return
		}
	}

	// Update the vanity host override if it changed. An empty string clears it
	// back to the derived host (the API accepts a non-nil pointer to "").
	if plan.URLSlug.ValueString() != state.URLSlug.ValueString() {
		_, err := r.client.UpdateAppURLSlug(ctx, appID, plan.URLSlug.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating app URL slug", err.Error())
			return
		}
	}

	// Update the database binding if it changed. An empty string clears it back
	// to the default (the project's sole database, or none when it has several).
	if plan.Database.ValueString() != state.Database.ValueString() {
		_, err := r.client.SetAppDatabase(ctx, appID, plan.Database.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating app database binding", err.Error())
			return
		}
	}

	// Apply a changed security context. Updated in place rather than replacing the
	// app: this block is how an app opts out of the non-root requirement, and
	// destroying and recreating an app to revoke an exception would make the
	// safer setting the more disruptive one. Removing the block clears it, which
	// the API expresses as a null security_context.
	if !plan.SecurityContext.Equal(state.SecurityContext) {
		sc := securityContextFromModel(ctx, plan.SecurityContext, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		_, err := r.client.SetAppSecurityContext(ctx, appID, sc)
		if err != nil {
			resp.Diagnostics.AddError("Error updating app security context", err.Error())
			return
		}
	}

	// Switch hosting mode if it changed, before scaling — replicas is only valid
	// on an always-on app, so scaling below needs the post-switch mode.
	if plan.Mode.ValueString() != state.Mode.ValueString() {
		_, err := r.client.SwitchMode(ctx, appID, plan.Mode.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error switching app mode", err.Error())
			return
		}
	}

	// Route carve-outs are validated against the app's mode and ingress, so this
	// runs after the mode switch above — a serverless→always-on move plus new
	// routes has to be applied in that order or the server rejects the routes.
	// An unknown plan value is not a request to change anything — sending it would
	// serialize to no routes and silently clear the carve-outs the app already has.
	if !plan.Routes.IsUnknown() && !plan.Routes.Equal(state.Routes) {
		_, err := r.client.UpdateAppRoutes(ctx, appID, routesFromModel(ctx, plan.Routes, &resp.Diagnostics))
		if resp.Diagnostics.HasError() {
			return
		}
		if err != nil {
			resp.Diagnostics.AddError("Error updating app routes", err.Error())
			return
		}
	}

	// Probes are validated against the app's mode, so like routes above this runs
	// after the mode switch. Also like routes, an unknown plan value is not a
	// request to change anything — sending it would clear the overrides the app
	// already has back to the shared health check.
	if !plan.Probes.IsUnknown() && !plan.Probes.Equal(state.Probes) {
		_, err := r.client.UpdateAppProbes(ctx, appID, probesFromModel(ctx, plan.Probes, &resp.Diagnostics))
		if resp.Diagnostics.HasError() {
			return
		}
		if err != nil {
			resp.Diagnostics.AddError("Error updating app probes", err.Error())
			return
		}
	}

	// Update scaling if any scaling attributes changed.
	if plan.MinScale.ValueInt64() != state.MinScale.ValueInt64() ||
		plan.MaxScale.ValueInt64() != state.MaxScale.ValueInt64() ||
		plan.Replicas.ValueInt64() != state.Replicas.ValueInt64() ||
		plan.CPULimit.ValueString() != state.CPULimit.ValueString() ||
		plan.MemoryLimit.ValueString() != state.MemoryLimit.ValueString() {

		minScale := int32(plan.MinScale.ValueInt64())
		maxScale := int32(plan.MaxScale.ValueInt64())
		replicas := int32(plan.Replicas.ValueInt64())
		scaleReq := client.ScaleRequest{
			MinScale:    &minScale,
			MaxScale:    &maxScale,
			CPULimit:    plan.CPULimit.ValueString(),
			MemoryLimit: plan.MemoryLimit.ValueString(),
		}
		// Replicas is an always-on setting; sending it on a serverless app is a 400.
		if plan.Mode.ValueString() != "serverless" {
			scaleReq.Replicas = &replicas
		}
		_, err := r.client.ScaleApp(ctx, appID, scaleReq)
		if err != nil {
			resp.Diagnostics.AddError("Error scaling app", err.Error())
			return
		}
	}

	// Grow persistent storage if the requested size changed (grow-only, enforced server-side).
	if plan.Storage.ValueString() != state.Storage.ValueString() && plan.Storage.ValueString() != "" {
		_, err := r.client.UpdateAppStorage(ctx, appID, plan.Storage.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating app storage", err.Error())
			return
		}
	}

	// Sync env vars: compute diff between old and new env/secret maps and update configs.
	var planEnv, stateEnv, planSecret, stateSecret map[string]string
	if !plan.Env.IsNull() && !plan.Env.IsUnknown() {
		resp.Diagnostics.Append(plan.Env.ElementsAs(ctx, &planEnv, false)...)
	}
	if !state.Env.IsNull() && !state.Env.IsUnknown() {
		resp.Diagnostics.Append(state.Env.ElementsAs(ctx, &stateEnv, false)...)
	}
	if !plan.Secret.IsNull() && !plan.Secret.IsUnknown() {
		resp.Diagnostics.Append(plan.Secret.ElementsAs(ctx, &planSecret, false)...)
	}
	if !state.Secret.IsNull() && !state.Secret.IsUnknown() {
		resp.Diagnostics.Append(state.Secret.ElementsAs(ctx, &stateSecret, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Remove old env keys no longer present.
	for k := range stateEnv {
		if _, exists := planEnv[k]; !exists {
			if err := r.client.UnsetConfig(ctx, appID, k); err != nil {
				resp.Diagnostics.AddError("Error unsetting env config", fmt.Sprintf("key %q: %s", k, err.Error()))
				return
			}
		}
	}
	// Set new/changed env keys.
	for k, v := range planEnv {
		if oldVal, exists := stateEnv[k]; !exists || oldVal != v {
			if _, err := r.client.SetConfig(ctx, appID, k, v, false); err != nil {
				resp.Diagnostics.AddError("Error setting env config", fmt.Sprintf("key %q: %s", k, err.Error()))
				return
			}
		}
	}
	// Remove old secret keys no longer present.
	for k := range stateSecret {
		if _, exists := planSecret[k]; !exists {
			if err := r.client.UnsetConfig(ctx, appID, k); err != nil {
				resp.Diagnostics.AddError("Error unsetting secret config", fmt.Sprintf("key %q: %s", k, err.Error()))
				return
			}
		}
	}
	// Set new/changed secret keys.
	for k, v := range planSecret {
		if oldVal, exists := stateSecret[k]; !exists || oldVal != v {
			if _, err := r.client.SetConfig(ctx, appID, k, v, true); err != nil {
				resp.Diagnostics.AddError("Error setting secret config", fmt.Sprintf("key %q: %s", k, err.Error()))
				return
			}
		}
	}

	// Update traffic if changed.
	if !plan.Traffic.IsNull() && !plan.Traffic.IsUnknown() {
		var trafficModels []TrafficTargetModel
		resp.Diagnostics.Append(plan.Traffic.ElementsAs(ctx, &trafficModels, false)...)
		if !resp.Diagnostics.HasError() && len(trafficModels) > 0 {
			targets := make([]client.TrafficTarget, len(trafficModels))
			for i, tm := range trafficModels {
				targets[i] = client.TrafficTarget{
					Revision: tm.Revision.ValueString(),
					Percent:  tm.Percent.ValueInt64(),
				}
			}

			result, err := r.client.SetTraffic(ctx, appID, targets)
			if err != nil {
				resp.Diagnostics.AddError("Error setting traffic", err.Error())
				return
			}
			r.setTrafficOnModel(ctx, &plan, result, &resp.Diagnostics)
		}
	}

	// Read the settled app and write THAT, rather than whichever call above
	// happened to run last. Every one of them answers with the app as it stood
	// mid-request — SwitchMode returns it while the new revision is still
	// rolling out — so the response that reached state depended on which
	// attributes the config changed, and a mode switch recorded a half-applied
	// app whose computed attributes never matched what the API then served
	// (fogpipe/cloud-workspace#226).
	app, err := r.client.GetApp(ctx, appID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading app", err.Error())
		return
	}

	r.setModelFromApp(&plan, app, &resp.Diagnostics)

	// Read current traffic if not already set by the update above.
	if plan.Traffic.IsNull() || plan.Traffic.IsUnknown() {
		targets, err := r.client.GetTraffic(ctx, appID)
		if err == nil {
			r.setTrafficOnModel(ctx, &plan, targets, &resp.Diagnostics)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AppResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteApp(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting app", err.Error())
	}
}

// ImportState accepts an app id (UUID) or a "project/name" pair where project
// is a project id or name. The id is tried first; on a miss the value is
// resolved as project/name via the list API.
func (r *AppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if _, err := r.client.GetApp(ctx, id); err == nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	} else if !isNotFound(err) {
		resp.Diagnostics.AddError("Error importing app", err.Error())
		return
	}

	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Error importing app",
			fmt.Sprintf("%q is not a known app id; import by name requires a \"project/name\" identifier", id),
		)
		return
	}
	app, err := r.findAppByName(ctx, parts[0], parts[1])
	if err != nil {
		resp.Diagnostics.AddError(
			"Error importing app",
			fmt.Sprintf("could not resolve app %q: %s", id, err.Error()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), app.ID)...)
}

// findAppByName resolves an app by name within a project. projectRef may be a
// project id or a project name.
func (r *AppResource) findAppByName(ctx context.Context, projectRef, name string) (*client.App, error) {
	projectID := projectRef
	if _, err := r.client.GetProject(ctx, projectRef); err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		projects, lerr := r.client.ListProjects(ctx)
		if lerr != nil {
			return nil, lerr
		}
		found := false
		for _, p := range projects {
			if p.Name == projectRef {
				projectID = p.ID
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("project %q is %w", projectRef, errNotAccessible)
		}
	}

	apps, err := r.client.ListApps(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, a := range apps {
		if a.Name == name {
			return a, nil
		}
	}
	return nil, fmt.Errorf("app %q in project %q is %w", name, projectRef, errNotAccessible)
}

// trafficTargetAttrTypes returns the attribute types for the traffic target object.
func trafficTargetAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"revision": types.StringType,
		"percent":  types.Int64Type,
		"url":      types.StringType,
	}
}

// setTrafficOnModel converts API traffic targets to the Terraform model.
func (r *AppResource) setTrafficOnModel(_ context.Context, model *AppResourceModel, targets []client.TrafficTarget, diags *diag.Diagnostics) {
	if len(targets) == 0 {
		model.Traffic = types.ListNull(types.ObjectType{AttrTypes: trafficTargetAttrTypes()})
		return
	}

	elems := make([]attr.Value, len(targets))
	for i, t := range targets {
		obj, d := types.ObjectValue(trafficTargetAttrTypes(), map[string]attr.Value{
			"revision": types.StringValue(t.Revision),
			"percent":  types.Int64Value(t.Percent),
			"url":      types.StringValue(t.URL),
		})
		diags.Append(d...)
		elems[i] = obj
	}
	list, d := types.ListValue(types.ObjectType{AttrTypes: trafficTargetAttrTypes()}, elems)
	diags.Append(d...)
	model.Traffic = list
}

// routeAttrTypes returns the attribute types for the route object.
func routeAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"path":       types.StringType,
		"visibility": types.StringType,
	}
}

// probeSchema builds one liveness/readiness/startup block. success_threshold is
// offered on all three because the schema is shared, but the API rejects a value
// above 1 on liveness and startup — Kubernetes requires it there.
func probeSchema(description string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: description + " Unset attributes fall back to the app's health_check_* values.",
		Optional:    true,
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Description: "HTTP path this probe requests, e.g. '/healthz'. Defaults to health_check_path.",
				Optional:    true,
			},
			"initial_delay_seconds": schema.Int64Attribute{
				Description: "Seconds to wait after the container starts before the first check. Defaults to 0.",
				Optional:    true,
			},
			"period_seconds": schema.Int64Attribute{
				Description: "Seconds between checks. Defaults to health_check_interval.",
				Optional:    true,
			},
			"timeout_seconds": schema.Int64Attribute{
				Description: "Seconds before a check counts as failed. Defaults to health_check_timeout.",
				Optional:    true,
			},
			"failure_threshold": schema.Int64Attribute{
				Description: "Consecutive failures before the probe acts. Defaults to health_check_retries.",
				Optional:    true,
			},
			"success_threshold": schema.Int64Attribute{
				Description: "Consecutive successes before the probe counts as passing. Readiness only — " +
					"Kubernetes requires 1 for liveness and startup, and the API rejects anything else there.",
				Optional: true,
			},
		},
	}
}

func probeSpecAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"path":                  types.StringType,
		"initial_delay_seconds": types.Int64Type,
		"period_seconds":        types.Int64Type,
		"timeout_seconds":       types.Int64Type,
		"failure_threshold":     types.Int64Type,
		"success_threshold":     types.Int64Type,
	}
}

func probesAttrTypes() map[string]attr.Type {
	spec := types.ObjectType{AttrTypes: probeSpecAttrTypes()}
	return map[string]attr.Type{"liveness": spec, "readiness": spec, "startup": spec}
}

// probeSpecToObject converts one probe from the API into state. The API omits a
// field it does not have, which decodes to the Go zero value — mapped back to
// null here, not to 0. Writing 0 where the config said nothing would report an
// inconsistent result after apply, and 0 is not what the field means: unset is
// "inherit the health check".
func probeSpecToObject(spec *client.ProbeSpec, diags *diag.Diagnostics) attr.Value {
	if spec == nil {
		return types.ObjectNull(probeSpecAttrTypes())
	}
	orNull := func(v int) attr.Value {
		if v == 0 {
			return types.Int64Null()
		}
		return types.Int64Value(int64(v))
	}
	path := types.StringNull()
	if spec.Path != "" {
		path = types.StringValue(spec.Path)
	}
	obj, d := types.ObjectValue(probeSpecAttrTypes(), map[string]attr.Value{
		"path":                  path,
		"initial_delay_seconds": orNull(spec.InitialDelaySeconds),
		"period_seconds":        orNull(spec.PeriodSeconds),
		"timeout_seconds":       orNull(spec.TimeoutSeconds),
		"failure_threshold":     orNull(spec.FailureThreshold),
		"success_threshold":     orNull(spec.SuccessThreshold),
	})
	diags.Append(d...)
	return obj
}

// setProbesOnModel converts the API's per-probe overrides to the Terraform model.
func setProbesOnModel(model *AppResourceModel, probes *client.ProbeOverrides, diags *diag.Diagnostics) {
	if probes == nil || (probes.Liveness == nil && probes.Readiness == nil && probes.Startup == nil) {
		model.Probes = types.ObjectNull(probesAttrTypes())
		return
	}
	obj, d := types.ObjectValue(probesAttrTypes(), map[string]attr.Value{
		"liveness":  probeSpecToObject(probes.Liveness, diags),
		"readiness": probeSpecToObject(probes.Readiness, diags),
		"startup":   probeSpecToObject(probes.Startup, diags),
	})
	diags.Append(d...)
	model.Probes = obj
}

// probeSpecFromObject converts one configured probe block into a client spec. A
// null block yields nil, which is what leaves that probe on the shared health
// check.
func probeSpecFromObject(ctx context.Context, o types.Object, diags *diag.Diagnostics) *client.ProbeSpec {
	if o.IsNull() || o.IsUnknown() {
		return nil
	}
	var m ProbeSpecModel
	diags.Append(o.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.ProbeSpec{
		Path:                m.Path.ValueString(),
		InitialDelaySeconds: int(m.InitialDelaySeconds.ValueInt64()),
		PeriodSeconds:       int(m.PeriodSeconds.ValueInt64()),
		TimeoutSeconds:      int(m.TimeoutSeconds.ValueInt64()),
		FailureThreshold:    int(m.FailureThreshold.ValueInt64()),
		SuccessThreshold:    int(m.SuccessThreshold.ValueInt64()),
	}
}

// probesFromModel converts the configured probes block into client overrides. A
// null or unknown block yields nil, which the update endpoint reads as "put all
// three back on the shared health check".
func probesFromModel(ctx context.Context, o types.Object, diags *diag.Diagnostics) *client.ProbeOverrides {
	if o.IsNull() || o.IsUnknown() {
		return nil
	}
	var m ProbeOverridesModel
	diags.Append(o.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	probes := &client.ProbeOverrides{
		Liveness:  probeSpecFromObject(ctx, m.Liveness, diags),
		Readiness: probeSpecFromObject(ctx, m.Readiness, diags),
		Startup:   probeSpecFromObject(ctx, m.Startup, diags),
	}
	if probes.Liveness == nil && probes.Readiness == nil && probes.Startup == nil {
		return nil
	}
	return probes
}

// setRoutesOnModel converts the API's route carve-outs to the Terraform model.
func setRoutesOnModel(model *AppResourceModel, routes []client.Route, diags *diag.Diagnostics) {
	if len(routes) == 0 {
		model.Routes = types.ListNull(types.ObjectType{AttrTypes: routeAttrTypes()})
		return
	}
	elems := make([]attr.Value, len(routes))
	for i, rt := range routes {
		obj, d := types.ObjectValue(routeAttrTypes(), map[string]attr.Value{
			"path":       types.StringValue(rt.Path),
			"visibility": types.StringValue(rt.Visibility),
		})
		diags.Append(d...)
		elems[i] = obj
	}
	list, d := types.ListValue(types.ObjectType{AttrTypes: routeAttrTypes()}, elems)
	diags.Append(d...)
	model.Routes = list
}

// routesFromModel converts the configured route list into client routes. An unset
// list yields nil (no carve-outs); an empty one yields an empty slice, which the
// update endpoint reads as "clear them all".
func routesFromModel(ctx context.Context, l types.List, diags *diag.Diagnostics) []client.Route {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var models []RouteModel
	diags.Append(l.ElementsAs(ctx, &models, false)...)
	routes := make([]client.Route, 0, len(models))
	for _, m := range models {
		// Visibility is Optional+Computed, so an omitted one arrives unknown on
		// create; internal is the server's default and the only reason to name a
		// route at all.
		visibility := "internal"
		if !m.Visibility.IsNull() && !m.Visibility.IsUnknown() && m.Visibility.ValueString() != "" {
			visibility = m.Visibility.ValueString()
		}
		routes = append(routes, client.Route{Path: m.Path.ValueString(), Visibility: visibility})
	}
	return routes
}

// setModelFromApp maps an API App response to the Terraform resource model.
// It preserves the plan's env/secret maps since they are not returned by the API.
func (r *AppResource) setModelFromApp(model *AppResourceModel, app *client.App, diags *diag.Diagnostics) {
	setRoutesOnModel(model, app.Routes, diags)
	setProbesOnModel(model, app.Probes, diags)
	// Everything the API echoes is read back, so a change made outside this
	// plan shows as drift (fogpipe/cloud-workspace#12). Each read preserves the
	// model's null where the API's "none" is indistinguishable from it, so a
	// round-trip of what was written is not a permanent diff.
	model.Command = stringListFromAPI(model.Command, app.Command)
	model.Args = stringListFromAPI(model.Args, app.Args)
	model.ReleaseCommand = stringListFromAPI(model.ReleaseCommand, app.ReleaseCommand)
	setVolumeMountsOnModel(model, app.VolumeMounts, diags)
	setSecurityContextOnModel(model, app.SecurityContext, diags)
	model.ID = types.StringValue(app.ID)
	model.ProjectID = types.StringValue(app.ProjectID)
	model.Name = types.StringValue(app.Name)
	model.DisplayName = types.StringValue(app.DisplayName)
	model.URLSlug = types.StringValue(app.URLSlug)
	model.Database = types.StringValue(app.DatabaseID)
	model.Image = types.StringValue(app.Image)
	model.Port = types.Int64Value(int64(app.Port))
	model.Ingress = types.StringValue(app.Ingress)
	model.Mode = types.StringValue(app.Mode)
	model.Type = types.StringValue(app.Type)
	model.Storage = types.StringValue(app.Storage)
	model.StoragePath = types.StringValue(app.StoragePath)
	model.Replicas = types.Int64Value(int64(app.Replicas))
	model.MinScale = types.Int64Value(int64(app.MinScale))
	model.MaxScale = types.Int64Value(int64(app.MaxScale))
	model.CPULimit = types.StringValue(app.CPULimit)
	model.MemoryLimit = types.StringValue(app.MemoryLimit)
	model.HealthCheckPath = types.StringValue(app.HealthCheckPath)
	model.HealthCheckTimeout = types.Int64Value(int64(app.HealthCheckTimeout))
	model.HealthCheckInterval = types.Int64Value(int64(app.HealthCheckInterval))
	model.HealthCheckRetries = types.Int64Value(int64(app.HealthCheckRetries))
	model.Status = types.StringValue(app.Status)
	model.URL = types.StringValue(app.URL)
	model.CreatedAt = types.StringValue(app.CreatedAt.String())
	model.UpdatedAt = types.StringValue(app.UpdatedAt.String())
	// Note: env and secret maps are preserved from the plan/state — not returned by API.
	// Note: service_account is preserved from the plan/state — the API returns service_account_id.
}

// stringListFromAPI reads a string list the API echoed. The API's "none" is an
// empty list, which a config expresses by leaving the attribute out — so an
// empty answer keeps the model's null (or its explicit empty list) rather than
// becoming a diff between null and [].
func stringListFromAPI(prior types.List, vals []string) types.List {
	if len(vals) == 0 {
		if !prior.IsNull() && !prior.IsUnknown() {
			return prior
		}
		return types.ListNull(types.StringType)
	}
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

func volumeMountAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"source":     types.StringType,
		"name":       types.StringType,
		"mount_path": types.StringType,
		"sub_path":   types.StringType,
	}
}

// setVolumeMountsOnModel reads the mounts the API echoed into the model. None
// reads as null, matching a config with no volume_mounts.
func setVolumeMountsOnModel(model *AppResourceModel, mounts []client.VolumeMount, diags *diag.Diagnostics) {
	if len(mounts) == 0 {
		model.VolumeMounts = types.ListNull(types.ObjectType{AttrTypes: volumeMountAttrTypes()})
		return
	}
	elems := make([]attr.Value, len(mounts))
	for i, m := range mounts {
		obj, d := types.ObjectValue(volumeMountAttrTypes(), map[string]attr.Value{
			"source":     types.StringValue(m.Source),
			"name":       optionalString(m.Name),
			"mount_path": types.StringValue(m.MountPath),
			"sub_path":   optionalString(m.SubPath),
		})
		diags.Append(d...)
		elems[i] = obj
	}
	list, d := types.ListValue(types.ObjectType{AttrTypes: volumeMountAttrTypes()}, elems)
	diags.Append(d...)
	model.VolumeMounts = list
}

func securityContextAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"run_as_user":               types.Int64Type,
		"run_as_group":              types.Int64Type,
		"fs_group":                  types.Int64Type,
		"run_as_non_root":           types.BoolType,
		"read_only_root_filesystem": types.BoolType,
	}
}

// setSecurityContextOnModel reads the security context the API echoed. The two
// booleans are omitted on the wire when false and default to false in the
// schema, so absent and false are the same value on both sides.
func setSecurityContextOnModel(model *AppResourceModel, sc *client.SecurityContext, diags *diag.Diagnostics) {
	if sc == nil {
		model.SecurityContext = types.ObjectNull(securityContextAttrTypes())
		return
	}
	optionalInt := func(v *int64) types.Int64 {
		if v == nil {
			return types.Int64Null()
		}
		return types.Int64Value(*v)
	}
	obj, d := types.ObjectValue(securityContextAttrTypes(), map[string]attr.Value{
		"run_as_user":               optionalInt(sc.RunAsUser),
		"run_as_group":              optionalInt(sc.RunAsGroup),
		"fs_group":                  optionalInt(sc.FSGroup),
		"run_as_non_root":           types.BoolValue(sc.RunAsNonRoot),
		"read_only_root_filesystem": types.BoolValue(sc.ReadOnlyRootFilesystem),
	})
	diags.Append(d...)
	model.SecurityContext = obj
}

// volumeMountsFromModel converts the Terraform volume_mounts list to client
// VolumeMount values. A null or unknown list yields nil (the field is omitted).
func volumeMountsFromModel(ctx context.Context, l types.List, diags *diag.Diagnostics) []client.VolumeMount {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var models []VolumeMountModel
	diags.Append(l.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return nil
	}
	out := make([]client.VolumeMount, len(models))
	for i, m := range models {
		out[i] = client.VolumeMount{
			Source:    m.Source.ValueString(),
			Name:      m.Name.ValueString(),
			MountPath: m.MountPath.ValueString(),
			SubPath:   m.SubPath.ValueString(),
		}
	}
	return out
}

// securityContextFromModel converts the Terraform security_context object to a
// client SecurityContext. A null or unknown object yields nil (image default).
func securityContextFromModel(ctx context.Context, o types.Object, diags *diag.Diagnostics) *client.SecurityContext {
	if o.IsNull() || o.IsUnknown() {
		return nil
	}
	var m SecurityContextModel
	diags.Append(o.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	sc := &client.SecurityContext{
		RunAsNonRoot:           m.RunAsNonRoot.ValueBool(),
		ReadOnlyRootFilesystem: m.ReadOnlyRootFilesystem.ValueBool(),
	}
	if !m.RunAsUser.IsNull() && !m.RunAsUser.IsUnknown() {
		v := m.RunAsUser.ValueInt64()
		sc.RunAsUser = &v
	}
	if !m.RunAsGroup.IsNull() && !m.RunAsGroup.IsUnknown() {
		v := m.RunAsGroup.ValueInt64()
		sc.RunAsGroup = &v
	}
	if !m.FSGroup.IsNull() && !m.FSGroup.IsUnknown() {
		v := m.FSGroup.ValueInt64()
		sc.FSGroup = &v
	}
	return sc
}

// stringListToSlice converts a Terraform string list to a Go slice. A null or
// unknown list yields nil (the request field is then omitted).
func stringListToSlice(ctx context.Context, l types.List, diags *diag.Diagnostics) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(l.ElementsAs(ctx, &out, false)...)
	return out
}

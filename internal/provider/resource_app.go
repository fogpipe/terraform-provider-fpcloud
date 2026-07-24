package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
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
	Image               types.String `tfsdk:"image"`
	Command             types.List   `tfsdk:"command"`
	Args                types.List   `tfsdk:"args"`
	ReleaseCommand      types.List   `tfsdk:"release_command"`
	VolumeMounts        types.List   `tfsdk:"volume_mounts"`
	SecurityContext     types.Object `tfsdk:"security_context"`
	Port                types.Int64  `tfsdk:"port"`
	Ingress             types.String `tfsdk:"ingress"`
	Mode                types.String `tfsdk:"mode"`
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
	AdoptExisting       types.Bool   `tfsdk:"adopt_existing"`
	Traffic             types.List   `tfsdk:"traffic"`
	Status              types.String `tfsdk:"status"`
	URL                 types.String `tfsdk:"url"`
	CreatedAt           types.String `tfsdk:"created_at"`
	UpdatedAt           types.String `tfsdk:"updated_at"`
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
			"image": schema.StringAttribute{
				Description: "Container image to deploy.",
				Required:    true,
			},
			"command": schema.ListAttribute{
				Description: "Container entrypoint override (ENTRYPOINT). Write-only from Terraform's " +
					"perspective — the API does not echo it back, so out-of-band changes are not detected.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"args": schema.ListAttribute{
				Description: "Container arguments (CMD/args). Write-only from Terraform's perspective — the " +
					"API does not echo it back, so out-of-band changes are not detected.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"release_command": schema.ListAttribute{
				Description: "Command run once per deploy — from the exact image being deployed, with the " +
					"app's env/secrets — before the new version goes live; a failure aborts the deploy " +
					"(e.g. DB migrations). A single element containing spaces runs via 'sh -c'; use " +
					"multiple elements for exec form. Write-only from Terraform's perspective.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"volume_mounts": schema.ListNestedAttribute{
				Description: "Mount a ConfigMap or Secret as read-only files, or an emptyDir as writable " +
					"scratch, at a container path. Create-only — the API has no update path and does not " +
					"echo these back, so any change forces the app to be replaced.",
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
					"seccomp) plus the run-as identity below. Create-only — the API has no update path and " +
					"does not echo it back, so any change forces the app to be replaced.",
				Optional: true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
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
						Description: "Require the container to run as a non-root user.",
						Optional:    true,
					},
					"read_only_root_filesystem": schema.BoolAttribute{
						Description: "Mount the container root filesystem read-only.",
						Optional:    true,
					},
				},
			},
			"port": schema.Int64Attribute{
				Description: "Container port. Defaults to 8080.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(8080),
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
				Description: "Environment variables (plaintext)",
			},
			"secret": schema.MapAttribute{
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "Secret environment variables (encrypted at rest)",
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
				Description: "HTTP path for health checks. Defaults to '/'. Set to a custom path (e.g. '/healthz') to enable startup probes.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("/"),
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
			"adopt_existing": schema.BoolAttribute{
				Description: "When true, if an app with this name already exists in the project, adopt it " +
					"into Terraform state on create instead of failing with a 409 conflict. Defaults to " +
					"false, so create never silently takes ownership of an app it did not create. Note: " +
					"adoption records the existing app in state but does not push the configured image/env/" +
					"secret — run a subsequent apply to reconcile them.",
				Optional: true,
			},
			"traffic": schema.ListNestedAttribute{
				Description: "Traffic routing configuration. Each block specifies a revision and its traffic percentage. Use '@latest' to route to the latest revision.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"revision": schema.StringAttribute{
							Description: "Revision name or '@latest' to route to the latest revision.",
							Required:    true,
						},
						"percent": schema.Int64Attribute{
							Description: "Traffic percentage (0-100). All percentages must sum to 100.",
							Required:    true,
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
				Description: "URL where the app is accessible.",
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

	// Build merged env vars from env + secret maps for the create request.
	envVars := make(map[string]string)
	var envMap, secretMap map[string]string

	if !plan.Env.IsNull() && !plan.Env.IsUnknown() {
		resp.Diagnostics.Append(plan.Env.ElementsAs(ctx, &envMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for k, v := range envMap {
			envVars[k] = v
		}
	}
	if !plan.Secret.IsNull() && !plan.Secret.IsUnknown() {
		resp.Diagnostics.Append(plan.Secret.ElementsAs(ctx, &secretMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for k, v := range secretMap {
			envVars[k] = v
		}
	}

	command := stringListToSlice(ctx, plan.Command, &resp.Diagnostics)
	args := stringListToSlice(ctx, plan.Args, &resp.Diagnostics)
	releaseCommand := stringListToSlice(ctx, plan.ReleaseCommand, &resp.Diagnostics)
	volumeMounts := volumeMountsFromModel(ctx, plan.VolumeMounts, &resp.Diagnostics)
	securityContext := securityContextFromModel(ctx, plan.SecurityContext, &resp.Diagnostics)
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
		Mode:                plan.Mode.ValueString(),
		Storage:             plan.Storage.ValueString(),
		StoragePath:         plan.StoragePath.ValueString(),
		EnvVars:             envVars,
		HealthCheckPath:     plan.HealthCheckPath.ValueString(),
		HealthCheckTimeout:  int(plan.HealthCheckTimeout.ValueInt64()),
		HealthCheckInterval: int(plan.HealthCheckInterval.ValueInt64()),
		HealthCheckRetries:  int(plan.HealthCheckRetries.ValueInt64()),
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
			r.setModelFromApp(&plan, existing)
			if targets, terr := r.client.GetTraffic(ctx, existing.ID); terr == nil {
				r.setTrafficOnModel(ctx, &plan, targets, &resp.Diagnostics)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
		resp.Diagnostics.AddError("Error creating app", err.Error())
		return
	}

	// Store env and secret values as app configs via the API.
	for k, v := range envMap {
		if _, err := r.client.SetConfig(ctx, app.ID, k, v, false); err != nil {
			resp.Diagnostics.AddError("Error setting env config", fmt.Sprintf("key %q: %s", k, err.Error()))
			return
		}
	}
	for k, v := range secretMap {
		if _, err := r.client.SetConfig(ctx, app.ID, k, v, true); err != nil {
			resp.Diagnostics.AddError("Error setting secret config", fmt.Sprintf("key %q: %s", k, err.Error()))
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

	r.setModelFromApp(&plan, app)
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

	r.setModelFromApp(&state, app)

	// Read current traffic.
	targets, err := r.client.GetTraffic(ctx, state.ID.ValueString())
	if err == nil {
		r.setTrafficOnModel(ctx, &state, targets, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AppResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var app *client.App
	appID := state.ID.ValueString()

	// Deploy new image if it changed.
	if plan.Image.ValueString() != state.Image.ValueString() {
		deployed, err := r.client.DeployApp(ctx, appID, client.DeployRequest{
			Image: plan.Image.ValueString(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Error deploying app", err.Error())
			return
		}
		app = deployed
	}

	// Update the cosmetic display name if it changed.
	if plan.DisplayName.ValueString() != state.DisplayName.ValueString() {
		renamed, err := r.client.UpdateAppDisplayName(ctx, appID, plan.DisplayName.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating app display name", err.Error())
			return
		}
		app = renamed
	}

	// Update the vanity host override if it changed. An empty string clears it
	// back to the derived host (the API accepts a non-nil pointer to "").
	if plan.URLSlug.ValueString() != state.URLSlug.ValueString() {
		reslugged, err := r.client.UpdateAppURLSlug(ctx, appID, plan.URLSlug.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating app URL slug", err.Error())
			return
		}
		app = reslugged
	}

	// Update the container command/args/release command if any changed. A
	// non-nil pointer (including an empty slice) replaces the value; an empty
	// slice clears the override back to the image defaults (or drops the
	// release phase).
	if !plan.Command.Equal(state.Command) || !plan.Args.Equal(state.Args) || !plan.ReleaseCommand.Equal(state.ReleaseCommand) {
		command := stringListToSlice(ctx, plan.Command, &resp.Diagnostics)
		args := stringListToSlice(ctx, plan.Args, &resp.Diagnostics)
		releaseCommand := stringListToSlice(ctx, plan.ReleaseCommand, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		updated, err := r.client.UpdateAppCommand(ctx, appID, &command, &args, &releaseCommand)
		if err != nil {
			resp.Diagnostics.AddError("Error updating app command", err.Error())
			return
		}
		app = updated
	}

	// Switch hosting mode if it changed, before scaling — replicas is only valid
	// on an always-on app, so scaling below needs the post-switch mode.
	if plan.Mode.ValueString() != state.Mode.ValueString() {
		switched, err := r.client.SwitchMode(ctx, appID, plan.Mode.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error switching app mode", err.Error())
			return
		}
		app = switched
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
		scaled, err := r.client.ScaleApp(ctx, appID, scaleReq)
		if err != nil {
			resp.Diagnostics.AddError("Error scaling app", err.Error())
			return
		}
		app = scaled
	}

	// Grow persistent storage if the requested size changed (grow-only, enforced server-side).
	if plan.Storage.ValueString() != state.Storage.ValueString() && plan.Storage.ValueString() != "" {
		grown, err := r.client.UpdateAppStorage(ctx, appID, plan.Storage.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating app storage", err.Error())
			return
		}
		app = grown
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

	// If neither deploy nor scale happened, re-read to get current state.
	if app == nil {
		fetched, err := r.client.GetApp(ctx, appID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading app", err.Error())
			return
		}
		app = fetched
	}

	r.setModelFromApp(&plan, app)

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

// setModelFromApp maps an API App response to the Terraform resource model.
// It preserves the plan's env/secret maps since they are not returned by the API.
func (r *AppResource) setModelFromApp(model *AppResourceModel, app *client.App) {
	model.ID = types.StringValue(app.ID)
	model.ProjectID = types.StringValue(app.ProjectID)
	model.Name = types.StringValue(app.Name)
	model.DisplayName = types.StringValue(app.DisplayName)
	model.URLSlug = types.StringValue(app.URLSlug)
	model.Image = types.StringValue(app.Image)
	model.Ingress = types.StringValue(app.Ingress)
	model.Mode = types.StringValue(app.Mode)
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
	// Note: command, args, and release_command are preserved from the plan/state — not returned by API.
	// Note: volume_mounts and security_context are preserved from the plan/state — not returned by API.
	// Note: service_account is preserved from the plan/state — the API returns service_account_id.
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

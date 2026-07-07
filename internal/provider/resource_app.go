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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	Image               types.String `tfsdk:"image"`
	Port                types.Int64  `tfsdk:"port"`
	Ingress             types.String `tfsdk:"ingress"`
	Tier                types.String `tfsdk:"tier"`
	Storage             types.String `tfsdk:"storage"`
	StoragePath         types.String `tfsdk:"storage_path"`
	ServiceAccount      types.String `tfsdk:"service_account"`
	Env                 types.Map    `tfsdk:"env"`
	Secret              types.Map    `tfsdk:"secret"`
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
				Description: "App name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"image": schema.StringAttribute{
				Description: "Container image to deploy.",
				Required:    true,
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
			"tier": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("dedicated"),
				Description: "Hosting tier: 'dedicated' (always-on Deployment, default) or 'serverless' (scale-to-zero Knative). Changing the tier replaces the app.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"storage": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Persistent volume size (e.g. '50Gi'). Opt-in and dedicated-tier only. Grow-only — the volume can never shrink.",
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
			"min_scale": schema.Int64Attribute{
				Description: "Minimum number of instances. Defaults to 1.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
			},
			"max_scale": schema.Int64Attribute{
				Description: "Maximum number of instances. Defaults to 10.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(10),
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

	createReq := client.CreateAppRequest{
		Name:                plan.Name.ValueString(),
		Image:               plan.Image.ValueString(),
		Port:                int(plan.Port.ValueInt64()),
		Ingress:             plan.Ingress.ValueString(),
		Tier:                plan.Tier.ValueString(),
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

	app, err := r.client.CreateApp(ctx, plan.ProjectID.ValueString(), createReq)
	if err != nil {
		if isConflict(err) && plan.AdoptExisting.ValueBool() {
			existing, ferr := r.findAppByName(ctx, plan.ProjectID.ValueString(), plan.Name.ValueString())
			if ferr != nil {
				resp.Diagnostics.AddError("Error adopting existing app", ferr.Error())
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

	// Apply scaling if non-default values were specified.
	minScale := int32(plan.MinScale.ValueInt64())
	maxScale := int32(plan.MaxScale.ValueInt64())
	cpuLimit := plan.CPULimit.ValueString()
	memoryLimit := plan.MemoryLimit.ValueString()

	if minScale != 1 || maxScale != 10 || cpuLimit != "500m" || memoryLimit != "512Mi" {
		scaled, err := r.client.ScaleApp(ctx, app.ID, client.ScaleRequest{
			MinScale:    &minScale,
			MaxScale:    &maxScale,
			CPULimit:    cpuLimit,
			MemoryLimit: memoryLimit,
		})
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
		// Read current traffic from API.
		targets, err := r.client.GetTraffic(ctx, app.ID)
		if err == nil {
			r.setTrafficOnModel(ctx, &plan, targets, &resp.Diagnostics)
		}
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

	// Update scaling if any scaling attributes changed.
	if plan.MinScale.ValueInt64() != state.MinScale.ValueInt64() ||
		plan.MaxScale.ValueInt64() != state.MaxScale.ValueInt64() ||
		plan.CPULimit.ValueString() != state.CPULimit.ValueString() ||
		plan.MemoryLimit.ValueString() != state.MemoryLimit.ValueString() {

		minScale := int32(plan.MinScale.ValueInt64())
		maxScale := int32(plan.MaxScale.ValueInt64())
		scaled, err := r.client.ScaleApp(ctx, appID, client.ScaleRequest{
			MinScale:    &minScale,
			MaxScale:    &maxScale,
			CPULimit:    plan.CPULimit.ValueString(),
			MemoryLimit: plan.MemoryLimit.ValueString(),
		})
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
			return nil, fmt.Errorf("no project named %q found", projectRef)
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
	return nil, fmt.Errorf("no app named %q found in project %q", name, projectRef)
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
	model.Image = types.StringValue(app.Image)
	model.Ingress = types.StringValue(app.Ingress)
	model.Tier = types.StringValue(app.Tier)
	model.Storage = types.StringValue(app.Storage)
	model.StoragePath = types.StringValue(app.StoragePath)
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

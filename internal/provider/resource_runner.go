package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                   = &RunnerResource{}
	_ resource.ResourceWithConfigure      = &RunnerResource{}
	_ resource.ResourceWithValidateConfig = &RunnerResource{}
	_ resource.ResourceWithImportState    = &RunnerResource{}
)

// NewRunnerResource returns a new managed GitHub Actions runner resource.
func NewRunnerResource() resource.Resource {
	return &RunnerResource{}
}

// RunnerResource defines the resource implementation.
type RunnerResource struct {
	client *client.Client
}

// RunnerResourceModel describes the resource data model.
type RunnerResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Project types.String `tfsdk:"project"`

	GitHubAccount   types.String `tfsdk:"github_account"`
	GitHubConfigURL types.String `tfsdk:"github_config_url"`
	RunnerGroup     types.String `tfsdk:"runner_group"`
	Size            types.String `tfsdk:"size"`
	MaxRunners      types.Int64  `tfsdk:"max_runners"`
	Builder         types.Object `tfsdk:"builder"`
	Services        types.List   `tfsdk:"services"`
	Credential      types.String `tfsdk:"credential"`

	GitHubAppID             types.String `tfsdk:"github_app_id"`
	GitHubAppInstallationID types.String `tfsdk:"github_app_installation_id"`
	GitHubAppPrivateKey     types.String `tfsdk:"github_app_private_key"`
	GitHubToken             types.String `tfsdk:"github_token"`

	Labels         types.List   `tfsdk:"labels"`
	Status         types.String `tfsdk:"status"`
	CurrentRunners types.Int64  `tfsdk:"current_runners"`
	RunningRunners types.Int64  `tfsdk:"running_runners"`
	PendingRunners types.Int64  `tfsdk:"pending_runners"`
}

var runnerSizes = []string{"small", "medium", "large"}

func (r *RunnerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runner"
}

func (r *RunnerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a project's GitHub Actions runner. A project has one: it is not a machine " +
			"but a scale set — a pod is created for one job and destroyed when it ends, so an idle " +
			"runner costs nothing. Workflows opt in with `runs-on: <project>-ci`. The runner serves " +
			"every repository in the GitHub account the project is connected to — connect it once " +
			"with `fpcloud github connect`, which proves you control that account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Runner ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description: "ID or name of the project this runner belongs to. A project has one runner, " +
					"so this is also what identifies it. Changing it forces a new runner.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"github_account": schema.StringAttribute{
				Description: "The GitHub account the runner serves, e.g. `acme`. Only with a credential " +
					"you supply (`app` or `token`), which carries no account of its own. With the " +
					"default `platform` credential the account comes from the project's GitHub " +
					"connection and setting this is an error — an account is proved, not named.",
				Optional: true,
			},
			"github_config_url": schema.StringAttribute{
				Description: "The account URL the runner registered with, derived from the connection " +
					"or from `github_account`. Read-only.",
				Computed: true,
			},
			"runner_group": schema.StringAttribute{
				Description: "GitHub runner group the runner joins. Defaults to `Default`.",
				Optional:    true,
				Computed:    true,
			},
			"size": schema.StringAttribute{
				Description: "What one job gets — the container your workflow's steps execute in — from a " +
					"fixed menu: `small` (1 CPU, 2Gi), `medium` (2 CPU, 4Gi) or `large` (4 CPU, 8Gi). " +
					"Defaults to `medium`. A job that exceeds its memory is killed rather than slowed, " +
					"and GitHub can take several minutes to notice, so a run that stalls with no output " +
					"and ends as cancelled is usually this. A builder, if you ask for one, is sized " +
					"separately and adds to what a job costs.",
				Optional: true,
				Computed: true,
			},
			"max_runners": schema.Int64Attribute{
				Description: "Jobs the runner runs at once; further jobs queue on GitHub. Defaults to 2. " +
					"Every one of them costs cores and memory for as long as it runs, so this is a " +
					"budget rather than a throughput dial.",
				Optional: true,
				Computed: true,
			},
			"builder": schema.SingleNestedAttribute{
				Description: "Run a rootless BuildKit alongside each job and point `BUILDKIT_HOST` at it. " +
					"There is no Docker daemon in a runner and Docker-in-Docker is not available, so this " +
					"is how a job builds images. Omit the block for a runner that builds nothing; set it to " +
					"`{}` for a builder at the platform's defaults. It is sized apart from the runner " +
					"because the two do different work — the runner's memory follows your workflow's " +
					"steps, the builder's follows your Dockerfile — and it adds to what a job costs.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"cpu": schema.StringAttribute{
						Description: "CPU limit for the builder, e.g. \"1\". Defaults to the platform's, which is not the runner's size.",
						Optional:    true,
						Computed:    true,
					},
					"memory": schema.StringAttribute{
						Description: "Memory limit for the builder, e.g. \"2Gi\". Defaults to the platform's, which is not the runner's size.",
						Optional:    true,
						Computed:    true,
					},
				},
			},
			"services": schema.ListNestedAttribute{
				Description: "Containers to run beside every job, reachable on `127.0.0.1` " +
					"from your steps. This is how a workflow gets a database or a cache here: a job's own " +
					"`services:` block does not work, because GitHub serves one by running the job inside " +
					"a container and there is no container mode on these runners. Declared on the runner, " +
					"they also coexist with `builder`, which a container mode would not. The set is " +
					"replaced whole, and every service counts towards your organization's ceiling for as " +
					"long as a job is running.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Container name in the job pod: a DNS-1123 label, unique within the runner. " +
								"It names nothing on the network — the containers share one.",
							Required: true,
						},
						"image": schema.StringAttribute{
							Description: "Image to run, e.g. `postgres:18-alpine`.",
							Required:    true,
						},
						"env": schema.MapAttribute{
							Description: "Environment for the container. NOT a secret store: it is stored and " +
								"read back as written, and it configures a container that lives for one job " +
								"and is reachable from nothing but that job's own pod — which is what GitHub " +
								"does with `services.*.env` too. A credential to anything that outlives the " +
								"job does not belong here.",
							ElementType: types.StringType,
							Optional:    true,
						},
						"cpu": schema.StringAttribute{
							Description: "CPU limit for this container, e.g. \"500m\". Defaults to the platform's " +
								"own, which is not the runner's size — a database beside a job has nothing to " +
								"do with how big the job is.",
							Optional: true,
							Computed: true,
						},
						"memory": schema.StringAttribute{
							Description: "Memory limit for this container, e.g. \"1Gi\". Defaults to the platform's own.",
							Optional:    true,
							Computed:    true,
						},
					},
				},
			},
			"credential": schema.StringAttribute{
				Description: "How the runner authenticates: `platform` (default) uses the Fogpipe GitHub App " +
					"and takes its account from the project's GitHub connection, so nothing else is set here; " +
					"`app` uses your own GitHub App; `token` uses a personal access token. Chosen explicitly " +
					"rather than inferred, so a runner that means to use the Fogpipe app and one carrying its own " +
					"key are told apart by reading the config.",
				Optional: true,
				Computed: true,
			},
			"github_app_id": schema.StringAttribute{
				Description: "Your GitHub App's id, with `credential = \"app\"`. Use alongside " +
					"`github_app_installation_id` and `github_app_private_key`.",
				Optional: true,
			},
			"github_app_installation_id": schema.StringAttribute{
				Description: "Installation id of your GitHub App on the organization, with `credential = \"app\"`. " +
					"With `credential = \"platform\"` it comes from the project's GitHub connection.",
				Optional: true,
				// Computed because the second sentence above is a value the
				// platform writes: under the platform credential the control
				// plane resolves the installation from the project's GitHub
				// connection and returns it. Optional alone declares that only
				// the practitioner writes this, and the framework rejects the
				// apply when a value appears where config had none.
				Computed: true,
			},
			"github_app_private_key": schema.StringAttribute{
				Description: "Your GitHub App's private key (PEM), with `credential = \"app\"`. Write-only — " +
					"never returned by the API; the configured value is preserved in state across reads.",
				Optional:  true,
				Sensitive: true,
			},
			"github_token": schema.StringAttribute{
				Description: "A personal access token, with `credential = \"token\"`. Write-only — never " +
					"returned by the API. It carries a person's full access and dies with their account.",
				Optional:  true,
				Sensitive: true,
			},
			"labels": schema.ListAttribute{
				Description: "The `runs-on` labels this runner answers to.",
				ElementType: types.StringType,
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Runner state: `pending` while it registers with GitHub, then `running`.",
				Computed:    true,
			},
			"current_runners": schema.Int64Attribute{
				Description: "Runners alive right now: `running_runners` plus `pending_runners`.",
				Computed:    true,
			},
			"running_runners": schema.Int64Attribute{
				Description: "Runners executing a job right now.",
				Computed:    true,
			},
			"pending_runners": schema.Int64Attribute{
				Description: "Runners that exist without a job — above all, waiting for a pod the org's ceiling refuses.",
				Computed:    true,
			},
		},
	}
}

func (r *RunnerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RunnerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg RunnerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Size.IsNull() || cfg.Size.IsUnknown() {
		return
	}
	for _, s := range runnerSizes {
		if cfg.Size.ValueString() == s {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		path.Root("size"),
		"Unknown runner size",
		fmt.Sprintf("size must be one of %v, got %q.", runnerSizes, cfg.Size.ValueString()),
	)
}

func (r *RunnerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RunnerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateRunnerRequest{
		GitHubAccount:           plan.GitHubAccount.ValueString(),
		RunnerGroup:             plan.RunnerGroup.ValueString(),
		Size:                    plan.Size.ValueString(),
		Builder:                 runnerBuilderFromModel(ctx, plan.Builder, &resp.Diagnostics),
		Services:                runnerServicesFromModel(ctx, plan.Services, &resp.Diagnostics),
		Credential:              plan.Credential.ValueString(),
		GitHubAppID:             plan.GitHubAppID.ValueString(),
		GitHubAppInstallationID: plan.GitHubAppInstallationID.ValueString(),
		GitHubAppPrivateKey:     plan.GitHubAppPrivateKey.ValueString(),
		GitHubToken:             plan.GitHubToken.ValueString(),
	}
	if !plan.MaxRunners.IsNull() && !plan.MaxRunners.IsUnknown() {
		v := int(plan.MaxRunners.ValueInt64())
		createReq.MaxRunners = &v
	}

	runner, err := r.client.CreateRunner(ctx, plan.Project.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating runner", err.Error())
		return
	}

	r.apply(ctx, &plan, runner, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RunnerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RunnerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	runner, err := r.client.GetRunner(ctx, state.Project.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading runner", err.Error())
		return
	}

	r.apply(ctx, &state, runner, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RunnerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RunnerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state RunnerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The whole mutable surface is sent every apply: the plan is the desired
	// state, so a field the user removed from the config must be cleared, not
	// left at whatever the server last saw.
	account := plan.GitHubAccount.ValueString()
	group := plan.RunnerGroup.ValueString()
	maxRunners := int(plan.MaxRunners.ValueInt64())

	updateReq := client.UpdateRunnerRequest{
		GitHubAccount: &account,
		RunnerGroup:   &group,
		MaxRunners:    &maxRunners,
	}
	// Size is computed when the config leaves it out, so an omitted size is
	// whatever the runner has, not a request to change it.
	if !plan.Size.IsNull() && !plan.Size.IsUnknown() {
		size := plan.Size.ValueString()
		updateReq.Size = &size
	}

	// The credential is sent only when the config changed it. The API never
	// returns it, so an unconditional send would re-encrypt and re-render the
	// same secret on every apply — and, worse, would clear it whenever it is
	// supplied from somewhere Terraform does not track.
	if !plan.Credential.Equal(state.Credential) ||
		!plan.GitHubAppID.Equal(state.GitHubAppID) ||
		!plan.GitHubAppInstallationID.Equal(state.GitHubAppInstallationID) ||
		!plan.GitHubAppPrivateKey.Equal(state.GitHubAppPrivateKey) ||
		!plan.GitHubToken.Equal(state.GitHubToken) {
		credential := plan.Credential.ValueString()
		updateReq.Credential = &credential
		appID := plan.GitHubAppID.ValueString()
		installationID := plan.GitHubAppInstallationID.ValueString()
		privateKey := plan.GitHubAppPrivateKey.ValueString()
		token := plan.GitHubToken.ValueString()
		updateReq.GitHubAppID = &appID
		updateReq.GitHubAppInstallationID = &installationID
		updateReq.GitHubAppPrivateKey = &privateKey
		updateReq.GitHubToken = &token
	}

	// The builder travels in the same patch as the runner's own size, because
	// the org's ceiling weighs the two together — a plan that shrinks both is
	// refused if it arrives as two requests. It is set only when the config
	// changed, so an unrelated apply does not re-render the runner's pods.
	if !plan.Builder.IsUnknown() && !plan.Builder.Equal(state.Builder) {
		builder := runnerBuilderFromModel(ctx, plan.Builder, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		updateReq.Builder, updateReq.NoBuilder = builder, builder == nil
	}
	// Same reasoning, same request: the set is replaced whole, and an empty
	// list is how a config says "none" — which is why this sends a non-nil
	// pointer to a possibly-empty slice rather than nil for both cases.
	if !plan.Services.IsUnknown() && !plan.Services.Equal(state.Services) {
		services := runnerServicesFromModel(ctx, plan.Services, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if services == nil {
			services = []client.RunnerService{}
		}
		updateReq.Services = &services
	}

	runner, err := r.client.UpdateRunner(ctx, state.Project.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating runner", err.Error())
		return
	}

	r.apply(ctx, &plan, runner, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RunnerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RunnerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteRunner(ctx, state.Project.ValueString()); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting runner", err.Error())
	}
}

func (r *RunnerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	runner, err := r.client.GetRunner(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing runner", err.Error())
		return
	}
	var state RunnerResourceModel
	state.Project = types.StringValue(runner.ProjectID)
	r.apply(ctx, &state, runner, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// apply maps an API Runner response onto the model.
//
// The credential fields are deliberately not touched. They are write-only, so
// the response carries nothing for them, and copying that nothing into state
// would make every subsequent plan show the configured key as an addition and
// re-send it forever.
func (r *RunnerResource) apply(ctx context.Context, m *RunnerResourceModel, runner *client.Runner, diags *diag.Diagnostics) {
	m.ID = types.StringValue(runner.ID)
	m.GitHubConfigURL = types.StringValue(runner.GitHubConfigURL)
	m.RunnerGroup = types.StringValue(runner.RunnerGroup)
	m.Size = types.StringValue(runner.Size)
	m.MaxRunners = types.Int64Value(int64(runner.MaxRunners))
	setRunnerBuilderOnModel(m, runner.Builder, diags)
	setRunnerServicesOnModel(ctx, m, runner.Services, diags)
	m.Credential = types.StringValue(runner.Credential)
	m.GitHubAppID = optionalString(runner.GitHubAppID)
	m.GitHubAppInstallationID = optionalString(runner.GitHubAppInstallationID)
	m.Status = types.StringValue(runner.Status)
	m.CurrentRunners = types.Int64Value(int64(runner.CurrentRunners))
	m.RunningRunners = types.Int64Value(int64(runner.RunningRunners))
	m.PendingRunners = types.Int64Value(int64(runner.PendingRunners))
	labels, d := types.ListValueFrom(ctx, types.StringType, runner.Labels)
	diags.Append(d...)
	m.Labels = labels
}

// runnerServiceAttrTypes is one service block's shape, needed to build a
// correctly typed null list when a runner declares none.
func runnerServiceAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":   types.StringType,
		"image":  types.StringType,
		"env":    types.MapType{ElemType: types.StringType},
		"cpu":    types.StringType,
		"memory": types.StringType,
	}
}

type runnerServiceModel struct {
	Name   types.String `tfsdk:"name"`
	Image  types.String `tfsdk:"image"`
	Env    types.Map    `tfsdk:"env"`
	CPU    types.String `tfsdk:"cpu"`
	Memory types.String `tfsdk:"memory"`
}

// runnerServicesFromModel converts the configured blocks into client services.
// A null or unknown list is a runner that declares none; an empty list is a
// runner that declares none ON PURPOSE, and the update path needs those to
// differ.
func runnerServicesFromModel(ctx context.Context, list types.List, diags *diag.Diagnostics) []client.RunnerService {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var models []runnerServiceModel
	diags.Append(list.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return nil
	}
	out := make([]client.RunnerService, 0, len(models))
	for _, m := range models {
		svc := client.RunnerService{
			Name:   m.Name.ValueString(),
			Image:  m.Image.ValueString(),
			CPU:    m.CPU.ValueString(),
			Memory: m.Memory.ValueString(),
		}
		if !m.Env.IsNull() && !m.Env.IsUnknown() {
			env := map[string]string{}
			diags.Append(m.Env.ElementsAs(ctx, &env, false)...)
			svc.Env = env
		}
		out = append(out, svc)
	}
	return out
}

// setRunnerServicesOnModel writes the API's services back. The sizes are
// Computed, so what a job costs is readable from `terraform show` rather than
// inferred from a default nobody wrote down — the same reason the builder's are.
func setRunnerServicesOnModel(ctx context.Context, m *RunnerResourceModel, services []client.RunnerService, diags *diag.Diagnostics) {
	elemType := types.ObjectType{AttrTypes: runnerServiceAttrTypes()}
	if len(services) == 0 {
		m.Services = types.ListNull(elemType)
		return
	}
	values := make([]attr.Value, 0, len(services))
	for _, svc := range services {
		env := types.MapNull(types.StringType)
		if len(svc.Env) > 0 {
			v, d := types.MapValueFrom(ctx, types.StringType, svc.Env)
			diags.Append(d...)
			env = v
		}
		obj, d := types.ObjectValue(runnerServiceAttrTypes(), map[string]attr.Value{
			"name":   types.StringValue(svc.Name),
			"image":  types.StringValue(svc.Image),
			"env":    env,
			"cpu":    types.StringValue(svc.CPU),
			"memory": types.StringValue(svc.Memory),
		})
		diags.Append(d...)
		values = append(values, obj)
	}
	list, d := types.ListValue(elemType, values)
	diags.Append(d...)
	m.Services = list
}

// runnerBuilderAttrTypes is the builder block's shape, needed to build a null
// object of the right type when a runner has none.
func runnerBuilderAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"cpu":    types.StringType,
		"memory": types.StringType,
	}
}

// runnerBuilderFromModel converts the configured builder block into a client
// builder. A null or unknown block is a runner that builds nothing.
func runnerBuilderFromModel(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.RunnerBuilder {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var model struct {
		CPU    types.String `tfsdk:"cpu"`
		Memory types.String `tfsdk:"memory"`
	}
	diags.Append(obj.As(ctx, &model, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.RunnerBuilder{CPU: model.CPU.ValueString(), Memory: model.Memory.ValueString()}
}

// setRunnerBuilderOnModel writes the API's builder back to the Terraform model.
// The sizes are Computed, so the server's resolved values land in state even
// when the config named none — which is the point: what a job costs is
// readable from `terraform show` rather than inferred from a default nobody
// wrote down.
func setRunnerBuilderOnModel(m *RunnerResourceModel, builder *client.RunnerBuilder, diags *diag.Diagnostics) {
	if builder == nil {
		m.Builder = types.ObjectNull(runnerBuilderAttrTypes())
		return
	}
	obj, d := types.ObjectValue(runnerBuilderAttrTypes(), map[string]attr.Value{
		"cpu":    types.StringValue(builder.CPU),
		"memory": types.StringValue(builder.Memory),
	})
	diags.Append(d...)
	m.Builder = obj
}

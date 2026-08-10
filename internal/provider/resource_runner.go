package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &RunnerResource{}
	_ resource.ResourceWithConfigure   = &RunnerResource{}
	_ resource.ResourceWithImportState = &RunnerResource{}
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
	ID          types.String `tfsdk:"id"`
	Project     types.String `tfsdk:"project"`
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`

	GitHubConfigURL types.String `tfsdk:"github_config_url"`
	RunnerGroup     types.String `tfsdk:"runner_group"`
	MinRunners      types.Int64  `tfsdk:"min_runners"`
	MaxRunners      types.Int64  `tfsdk:"max_runners"`
	Image           types.String `tfsdk:"image"`
	CPU             types.String `tfsdk:"cpu"`
	Memory          types.String `tfsdk:"memory"`
	Builds          types.Bool   `tfsdk:"builds"`

	GitHubAppID             types.String `tfsdk:"github_app_id"`
	GitHubAppInstallationID types.String `tfsdk:"github_app_installation_id"`
	GitHubAppPrivateKey     types.String `tfsdk:"github_app_private_key"`
	GitHubToken             types.String `tfsdk:"github_token"`

	Labels         types.List   `tfsdk:"labels"`
	Status         types.String `tfsdk:"status"`
	CurrentRunners types.Int64  `tfsdk:"current_runners"`
}

func (r *RunnerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runner"
}

func (r *RunnerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a pool of GitHub Actions runners in a project. A pool is not a machine: " +
			"a pod is created for one job and destroyed when it ends, so an idle pool costs nothing. " +
			"Workflows opt in by naming the pool in `runs-on`. Point `github_config_url` at an " +
			"organization to serve every repository in it, or at a single repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Runner ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description: "ID or name of the project this runner belongs to. Changing it forces a new runner.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Runner name, unique within the project (DNS-1123 label). This is also the " +
					"`runs-on` label workflows use. Changing it forces a new runner.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable label. Defaults to the name. Mutable in place.",
				Optional:    true,
				Computed:    true,
			},
			"github_config_url": schema.StringAttribute{
				Description: "The organization (`https://github.com/acme`) or repository " +
					"(`https://github.com/acme/api`) the pool serves. This is the pool's entire scope.",
				Required: true,
			},
			"runner_group": schema.StringAttribute{
				Description: "GitHub runner group the pool joins. Defaults to `Default`.",
				Optional:    true,
				Computed:    true,
			},
			"min_runners": schema.Int64Attribute{
				Description: "Runners kept idle and ready. Defaults to 0 — the pool scales to zero and a " +
					"job waits a few seconds for its pod.",
				Optional: true,
				Computed: true,
			},
			"max_runners": schema.Int64Attribute{
				Description: "Jobs the pool runs at once; further jobs queue on GitHub. Defaults to 2. " +
					"Runners share the project's quota with everything else in it.",
				Optional: true,
				Computed: true,
			},
			"image": schema.StringAttribute{
				Description: "Runner image. Defaults to the platform's, which the operator keeps current — " +
					"GitHub refuses work to deprecated runner versions, so pinning your own means keeping it current yourself.",
				Optional: true,
			},
			"cpu": schema.StringAttribute{
				Description: "CPU limit for one runner pod, e.g. \"2\".",
				Optional:    true,
			},
			"memory": schema.StringAttribute{
				Description: "Memory limit for one runner pod, e.g. \"4Gi\".",
				Optional:    true,
			},
			"builds": schema.BoolAttribute{
				Description: "Run a rootless BuildKit alongside each job and point `BUILDKIT_HOST` at it. " +
					"There is no Docker daemon in a runner and Docker-in-Docker is not available, so this is " +
					"how a job builds images. Off by default.",
				Optional: true,
				Computed: true,
			},
			"github_app_id": schema.StringAttribute{
				Description: "GitHub App id the pool authenticates as. Use this with " +
					"`github_app_installation_id` and `github_app_private_key`, or use `github_token` — never both.",
				Optional: true,
			},
			"github_app_installation_id": schema.StringAttribute{
				Description: "Installation id of the GitHub App on the organization.",
				Optional:    true,
			},
			"github_app_private_key": schema.StringAttribute{
				Description: "The GitHub App's private key (PEM). Write-only — never returned by the API; " +
					"the configured value is preserved in state across reads.",
				Optional:  true,
				Sensitive: true,
			},
			"github_token": schema.StringAttribute{
				Description: "A personal access token, instead of a GitHub App. Write-only — never returned " +
					"by the API. It carries a person's full access and dies with their account; prefer an App.",
				Optional:  true,
				Sensitive: true,
			},
			"labels": schema.ListAttribute{
				Description: "The `runs-on` labels this pool answers to.",
				ElementType: types.StringType,
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Pool state: `pending` while it registers with GitHub, then `running`.",
				Computed:    true,
			},
			"current_runners": schema.Int64Attribute{
				Description: "Runner pods alive right now — how many of your jobs are running.",
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

func (r *RunnerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RunnerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateRunnerRequest{
		Name:                    plan.Name.ValueString(),
		DisplayName:             plan.DisplayName.ValueString(),
		GitHubConfigURL:         plan.GitHubConfigURL.ValueString(),
		RunnerGroup:             plan.RunnerGroup.ValueString(),
		Image:                   plan.Image.ValueString(),
		CPU:                     plan.CPU.ValueString(),
		Memory:                  plan.Memory.ValueString(),
		Builds:                  plan.Builds.ValueBool(),
		GitHubAppID:             plan.GitHubAppID.ValueString(),
		GitHubAppInstallationID: plan.GitHubAppInstallationID.ValueString(),
		GitHubAppPrivateKey:     plan.GitHubAppPrivateKey.ValueString(),
		GitHubToken:             plan.GitHubToken.ValueString(),
	}
	if !plan.MinRunners.IsNull() && !plan.MinRunners.IsUnknown() {
		v := int(plan.MinRunners.ValueInt64())
		createReq.MinRunners = &v
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

	runner, err := r.client.GetRunner(ctx, state.ID.ValueString())
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
	displayName := plan.DisplayName.ValueString()
	configURL := plan.GitHubConfigURL.ValueString()
	group := plan.RunnerGroup.ValueString()
	image := plan.Image.ValueString()
	cpu := plan.CPU.ValueString()
	memory := plan.Memory.ValueString()
	minRunners := int(plan.MinRunners.ValueInt64())
	maxRunners := int(plan.MaxRunners.ValueInt64())
	builds := plan.Builds.ValueBool()

	updateReq := client.UpdateRunnerRequest{
		DisplayName:     &displayName,
		GitHubConfigURL: &configURL,
		RunnerGroup:     &group,
		MinRunners:      &minRunners,
		MaxRunners:      &maxRunners,
		Image:           &image,
		CPU:             &cpu,
		Memory:          &memory,
		Builds:          &builds,
	}

	// The credential is sent only when the config changed it. The API never
	// returns it, so an unconditional send would re-encrypt and re-render the
	// same secret on every apply — and, worse, would clear it whenever it is
	// supplied from somewhere Terraform does not track.
	if !plan.GitHubAppID.Equal(state.GitHubAppID) ||
		!plan.GitHubAppInstallationID.Equal(state.GitHubAppInstallationID) ||
		!plan.GitHubAppPrivateKey.Equal(state.GitHubAppPrivateKey) ||
		!plan.GitHubToken.Equal(state.GitHubToken) {
		appID := plan.GitHubAppID.ValueString()
		installationID := plan.GitHubAppInstallationID.ValueString()
		privateKey := plan.GitHubAppPrivateKey.ValueString()
		token := plan.GitHubToken.ValueString()
		updateReq.GitHubAppID = &appID
		updateReq.GitHubAppInstallationID = &installationID
		updateReq.GitHubAppPrivateKey = &privateKey
		updateReq.GitHubToken = &token
	}

	runner, err := r.client.UpdateRunner(ctx, state.ID.ValueString(), updateReq)
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

	if err := r.client.DeleteRunner(ctx, state.ID.ValueString()); err != nil {
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
	m.Name = types.StringValue(runner.Name)
	m.DisplayName = types.StringValue(runner.DisplayName)
	m.GitHubConfigURL = types.StringValue(runner.GitHubConfigURL)
	m.RunnerGroup = types.StringValue(runner.RunnerGroup)
	m.MinRunners = types.Int64Value(int64(runner.MinRunners))
	m.MaxRunners = types.Int64Value(int64(runner.MaxRunners))
	m.Builds = types.BoolValue(runner.Builds)
	m.Image = optionalString(runner.Image)
	m.CPU = optionalString(runner.CPU)
	m.Memory = optionalString(runner.Memory)
	m.GitHubAppID = optionalString(runner.GitHubAppID)
	m.GitHubAppInstallationID = optionalString(runner.GitHubAppInstallationID)
	m.Status = types.StringValue(runner.Status)
	m.CurrentRunners = types.Int64Value(int64(runner.CurrentRunners))
	labels, d := types.ListValueFrom(ctx, types.StringType, runner.Labels)
	diags.Append(d...)
	m.Labels = labels
}

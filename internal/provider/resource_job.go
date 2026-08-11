package provider

import (
	"context"
	"fmt"

	"github.com/fogpipe/cloud-cli/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &JobResource{}
	_ resource.ResourceWithConfigure   = &JobResource{}
	_ resource.ResourceWithImportState = &JobResource{}
)

// NewJobResource returns a new scheduled job resource.
func NewJobResource() resource.Resource {
	return &JobResource{}
}

// JobResource defines the resource implementation.
type JobResource struct {
	client *client.Client
}

// JobResourceModel describes the resource data model.
type JobResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Project     types.String `tfsdk:"project"`
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`
	App         types.String `tfsdk:"app"`

	Schedule    types.String `tfsdk:"schedule"`
	Timezone    types.String `tfsdk:"timezone"`
	Concurrency types.String `tfsdk:"concurrency"`
	MaxRetries  types.Int64  `tfsdk:"max_retries"`
	Timeout     types.Int64  `tfsdk:"timeout_seconds"`
	KeepRuns    types.Int64  `tfsdk:"keep_runs"`

	RetainSucceeded types.Int64 `tfsdk:"retain_succeeded_seconds"`
	RetainFailed    types.Int64 `tfsdk:"retain_failed_seconds"`
	Suspended       types.Bool  `tfsdk:"suspended"`

	Image       types.String `tfsdk:"image"`
	Command     types.List   `tfsdk:"command"`
	Args        types.List   `tfsdk:"args"`
	HTTPURL     types.String `tfsdk:"http_url"`
	HTTPMethod  types.String `tfsdk:"http_method"`
	HTTPHeaders types.Map    `tfsdk:"http_headers"`
	HTTPBody    types.String `tfsdk:"http_body"`

	Target types.String `tfsdk:"target"`
}

func (r *JobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_job"
}

func (r *JobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a scheduled job: one cron schedule plus what it runs — an HTTP request " +
			"to an endpoint (no image needed) or a container. Setting `app` makes the job inherit that " +
			"app's image, config and secrets, so an authenticated self-call needs no second copy of a " +
			"token: `$VAR` in the URL, headers or body is resolved from the app's config when the job " +
			"fires. A relative `http_url` resolves to the app's in-cluster address, so the request " +
			"never leaves the cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Job ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Description: "ID or name of the project this job belongs to. Changing it forces a new job.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Job name, unique within the project (DNS-1123 label). Changing it forces a new job.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "Human-readable label. Defaults to the name. Mutable in place.",
				Optional:    true,
				Computed:    true,
			},
			"app": schema.StringAttribute{
				Description: "Name or ID of the app whose image, config, secrets and identity the job " +
					"inherits. Changing it forces a new job.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schedule": schema.StringAttribute{
				Description: "Cron schedule, e.g. \"*/15 * * * *\". Mutable in place.",
				Required:    true,
			},
			"timezone": schema.StringAttribute{
				Description: "tz database name the schedule is read in, e.g. \"Europe/Stockholm\". " +
					"Defaults to UTC — set it for wall-clock schedules that must survive daylight saving.",
				Optional: true,
				Computed: true,
			},
			"concurrency": schema.StringAttribute{
				Description: "What happens when a run is still going at the next fire: `forbid` (the " +
					"default — skip), `replace` (kill and restart), or `allow` (run in parallel).",
				Optional: true,
				Computed: true,
			},
			"max_retries": schema.Int64Attribute{
				Description: "Retries after a failed run (default 3). The delay between attempts is an " +
					"exponential backoff from 10s, capped at 6 minutes.",
				Optional: true,
				Computed: true,
			},
			"timeout_seconds": schema.Int64Attribute{
				Description: "Seconds a single run may take before it is killed and counted as failed " +
					"(default 300, max 43200).",
				Optional: true,
				Computed: true,
			},
			"keep_runs": schema.Int64Attribute{
				Description: "How many runs are retained in the history, newest first (default 10). " +
					"Applies to both outcomes; combined with the retention windows below, a run is " +
					"dropped once either bound is exceeded.",
				Optional: true,
				Computed: true,
			},
			"retain_succeeded_seconds": schema.Int64Attribute{
				Description: "How long a completed run is kept, in seconds (default 604800 = 7 days). " +
					"0 removes the age limit, leaving `keep_runs` the only bound.",
				Optional: true,
				Computed: true,
			},
			"retain_failed_seconds": schema.Int64Attribute{
				Description: "How long an errored run is kept, in seconds (default 2592000 = 30 days). " +
					"Kept separately from completed runs because a failure stays worth reading long " +
					"after a success is noise. 0 removes the age limit.",
				Optional: true,
				Computed: true,
			},
			"suspended": schema.BoolAttribute{
				Description: "Pause the schedule. A suspended job keeps its history and can still be " +
					"triggered manually.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"image": schema.StringAttribute{
				Description: "Container image to run. Defaults to the `app` image when an app is set; " +
					"leave both unset only for an HTTP job.",
				Optional: true,
				Computed: true,
			},
			"command": schema.ListAttribute{
				Description: "Entrypoint override for a container job. A single element containing " +
					"spaces runs through `sh -c`.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"args": schema.ListAttribute{
				Description: "Arguments for a container job.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"http_url": schema.StringAttribute{
				Description: "Request an endpoint instead of running an image: an absolute URL, or a " +
					"path like \"/internal/sweep\" resolved against the job's `app`. Setting this makes " +
					"the job an HTTP job.",
				Optional: true,
			},
			"http_method": schema.StringAttribute{
				Description: "HTTP method for `http_url` (default POST).",
				Optional:    true,
				Computed:    true,
			},
			"http_headers": schema.MapAttribute{
				Description: "Headers sent with `http_url`. `$VAR` is resolved from the app's config " +
					"when the job fires, so a token stays in the config store.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"http_body": schema.StringAttribute{
				Description: "Request body for `http_url`. Supports the same `$VAR` expansion as headers.",
				Optional:    true,
			},
			"target": schema.StringAttribute{
				Description: "Resolved target type: `http` when `http_url` is set, otherwise `container`.",
				Computed:    true,
			},
		},
	}
}

func (r *JobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *JobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan JobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.CreateJobRequest{
		Name:        plan.Name.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
		App:         plan.App.ValueString(),
		Schedule:    plan.Schedule.ValueString(),
		Timezone:    plan.Timezone.ValueString(),
		Concurrency: plan.Concurrency.ValueString(),
		Suspended:   plan.Suspended.ValueBool(),
		Image:       plan.Image.ValueString(),
		Command:     stringListValues(ctx, plan.Command, &resp.Diagnostics),
		Args:        stringListValues(ctx, plan.Args, &resp.Diagnostics),
		HTTPURL:     plan.HTTPURL.ValueString(),
		HTTPMethod:  plan.HTTPMethod.ValueString(),
		HTTPHeaders: stringMapValues(ctx, plan.HTTPHeaders, &resp.Diagnostics),
		HTTPBody:    plan.HTTPBody.ValueString(),
	}
	if !plan.MaxRetries.IsNull() && !plan.MaxRetries.IsUnknown() {
		v := int(plan.MaxRetries.ValueInt64())
		createReq.MaxRetries = &v
	}
	if !plan.Timeout.IsNull() && !plan.Timeout.IsUnknown() {
		v := int(plan.Timeout.ValueInt64())
		createReq.Timeout = &v
	}
	if !plan.KeepRuns.IsNull() && !plan.KeepRuns.IsUnknown() {
		v := int(plan.KeepRuns.ValueInt64())
		createReq.KeepRuns = &v
	}
	if !plan.RetainSucceeded.IsNull() && !plan.RetainSucceeded.IsUnknown() {
		v := int(plan.RetainSucceeded.ValueInt64())
		createReq.RetainSucceeded = &v
	}
	if !plan.RetainFailed.IsNull() && !plan.RetainFailed.IsUnknown() {
		v := int(plan.RetainFailed.ValueInt64())
		createReq.RetainFailed = &v
	}
	if resp.Diagnostics.HasError() {
		return
	}

	job, err := r.client.CreateJob(ctx, plan.Project.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating job", err.Error())
		return
	}

	r.apply(&plan, job)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *JobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state JobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	job, err := r.client.GetJob(ctx, state.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading job", err.Error())
		return
	}

	r.apply(&state, job)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *JobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan JobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state JobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The whole mutable surface is sent every apply: the plan is the desired
	// state, so a field the user removed from the config must be cleared, not
	// left at whatever the server last saw.
	displayName := plan.DisplayName.ValueString()
	schedule := plan.Schedule.ValueString()
	timezone := plan.Timezone.ValueString()
	concurrency := plan.Concurrency.ValueString()
	image := plan.Image.ValueString()
	httpURL := plan.HTTPURL.ValueString()
	httpMethod := plan.HTTPMethod.ValueString()
	httpBody := plan.HTTPBody.ValueString()
	maxRetries := int(plan.MaxRetries.ValueInt64())
	timeout := int(plan.Timeout.ValueInt64())
	keepRuns := int(plan.KeepRuns.ValueInt64())
	retainSucceeded := int(plan.RetainSucceeded.ValueInt64())
	retainFailed := int(plan.RetainFailed.ValueInt64())
	suspended := plan.Suspended.ValueBool()
	command := stringListValues(ctx, plan.Command, &resp.Diagnostics)
	args := stringListValues(ctx, plan.Args, &resp.Diagnostics)
	headers := stringMapValues(ctx, plan.HTTPHeaders, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := client.UpdateJobRequest{
		DisplayName: &displayName,
		Schedule:    &schedule,
		Timezone:    &timezone,
		Concurrency: &concurrency,
		MaxRetries:  &maxRetries,
		Timeout:     &timeout,
		KeepRuns:    &keepRuns,

		RetainSucceeded: &retainSucceeded,
		RetainFailed:    &retainFailed,
		Suspended:       &suspended,
		Image:           &image,
		Command:         &command,
		Args:            &args,
		HTTPURL:         &httpURL,
		HTTPMethod:      &httpMethod,
		HTTPHeaders:     &headers,
		HTTPBody:        &httpBody,
	}

	job, err := r.client.UpdateJob(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating job", err.Error())
		return
	}

	r.apply(&plan, job)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *JobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state JobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteJob(ctx, state.ID.ValueString()); err != nil {
		if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Error deleting job", err.Error())
	}
}

func (r *JobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	job, err := r.client.GetJob(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing job", err.Error())
		return
	}
	var state JobResourceModel
	state.Project = types.StringValue(job.ProjectID)
	r.apply(&state, job)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// apply maps an API Job response onto the model. Optional attributes that the
// server echoes back empty stay null, so an unset field doesn't show as drift.
func (r *JobResource) apply(m *JobResourceModel, job *client.Job) {
	m.ID = types.StringValue(job.ID)
	m.Name = types.StringValue(job.Name)
	m.DisplayName = types.StringValue(job.DisplayName)
	m.Schedule = types.StringValue(job.Schedule)
	m.Timezone = types.StringValue(job.Timezone)
	m.Concurrency = types.StringValue(job.Concurrency)
	m.MaxRetries = types.Int64Value(int64(job.MaxRetries))
	m.Timeout = types.Int64Value(int64(job.Timeout))
	m.KeepRuns = types.Int64Value(int64(job.KeepRuns))
	m.RetainSucceeded = types.Int64Value(int64(job.RetainSucceeded))
	m.RetainFailed = types.Int64Value(int64(job.RetainFailed))
	m.Suspended = types.BoolValue(job.Suspended)
	m.Target = types.StringValue(job.Target)
	m.Image = types.StringValue(job.Image)
	m.HTTPURL = optionalString(job.HTTPURL)
	m.HTTPBody = optionalString(job.HTTPBody)
	if job.Target == client.JobTargetHTTP {
		m.HTTPMethod = types.StringValue(job.HTTPMethod)
	} else {
		m.HTTPMethod = types.StringNull()
	}
	if m.Project.IsNull() || m.Project.IsUnknown() {
		m.Project = types.StringValue(job.ProjectID)
	}
	if job.AppName != "" && (m.App.IsNull() || m.App.IsUnknown()) {
		m.App = types.StringValue(job.AppName)
	}
}

// stringListValues reads a list attribute into a plain slice; a null/unknown
// list yields nil, which the API reads as "unset".
func stringListValues(ctx context.Context, list types.List, diags *diag.Diagnostics) []string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out
}

// stringMapValues reads a map attribute into a plain map; a null/unknown map
// yields nil.
func stringMapValues(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := map[string]string{}
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	return out
}

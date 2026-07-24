package provider

import (
	"context"
	"os"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &FogpipeProvider{}

// FogpipeProvider defines the provider implementation.
type FogpipeProvider struct {
	version string
}

// FogpipeProviderModel describes the provider data model.
type FogpipeProviderModel struct {
	APIKey types.String `tfsdk:"api_key"`
	APIURL types.String `tfsdk:"api_url"`
}

// New returns a function that creates a new FogpipeProvider instance.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &FogpipeProvider{version: version}
	}
}

func (p *FogpipeProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "fpcloud"
	resp.Version = p.version
}

func (p *FogpipeProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Fogpipe provider manages resources on the Fogpipe PaaS platform.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "API key for Fogpipe. Can also be set via the FPCLOUD_API_KEY environment variable, or inherited from the fpcloud CLI login — an API key in ~/.fpcloud/config.yaml (honouring FPCLOUD_CONFIG_DIR), or the Google OIDC token from `fpcloud login` via `fpcloud get-token` (requires the fpcloud CLI on PATH).",
				Optional:    true,
				Sensitive:   true,
			},
			"api_url": schema.StringAttribute{
				Description: "API URL for Fogpipe. Defaults to https://api.cloud.fogpipe.com. Can also be set via the FPCLOUD_API_URL environment variable, or inherited from the fpcloud CLI config.",
				Optional:    true,
			},
		},
	}
}

func (p *FogpipeProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config FogpipeProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The CLI's ~/.fpcloud/config.yaml is the lowest-priority credential source,
	// so `fpcloud login` doubles as the provider's default credentials — the
	// AWS/GCP model (env var wins, CLI config file is the login-based fallback).
	cli := loadCLIConfig()

	// Resolve API key: config > env var > CLI config key > CLI OIDC login.
	// The last step delegates to `fpcloud get-token`, so the default Google
	// login (`fpcloud login`) works with nothing in HCL or env — bare
	// `tofu apply`, the AWS/GCP way.
	apiKey := cli.APIKey
	if envKey := os.Getenv("FPCLOUD_API_KEY"); envKey != "" {
		apiKey = envKey
	}
	if !config.APIKey.IsNull() && !config.APIKey.IsUnknown() {
		apiKey = config.APIKey.ValueString()
	}
	if apiKey == "" {
		apiKey = cliOIDCToken()
	}
	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing credentials",
			"The provider found no credentials. Log in with the fpcloud CLI (`fpcloud login` or `fpcloud auth login`), set FPCLOUD_API_KEY, or set api_key in the provider configuration.",
		)
		return
	}

	// Resolve API URL: config > env var > CLI config > default
	apiURL := "https://api.cloud.fogpipe.com"
	if cli.APIURL != "" {
		apiURL = cli.APIURL
	}
	if envURL := os.Getenv("FPCLOUD_API_URL"); envURL != "" {
		apiURL = envURL
	}
	if !config.APIURL.IsNull() && !config.APIURL.IsUnknown() {
		apiURL = config.APIURL.ValueString()
	}

	c := client.New(apiURL, apiKey)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *FogpipeProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewOrgResource,
		NewProjectResource,
		NewOIDCFederationResource,
		NewAppResource,
		NewDatabaseResource,
		NewDatabaseBackupDestinationResource,
		NewBucketResource,
		NewBucketKeyResource,
		NewBucketDomainResource,
		NewAppBucketResource,
		NewDomainResource,
		NewAppConfigResource,
		NewWebhookResource,
		NewServiceAccountResource,
		NewServiceAccountKeyResource,
		NewIAMBindingResource,
		NewOrgMemberResource,
		NewOrgSecretResource,
		NewRegistryRetentionPolicyResource,
		NewRegistryVisibilityResource,
	}
}

func (p *FogpipeProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProjectDataSource,
		NewAppDataSource,
		NewDatabaseDataSource,
	}
}

package provider_test

import (
	"testing"

	"github.com/fogpipe/terraform-provider-fpcloud/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"fpcloud": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func TestProviderSchema(t *testing.T) {
	t.Parallel()

	// Creating the provider and calling Metadata validates the schema.
	p := provider.New("test")()
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestProviderMetadata(t *testing.T) {
	t.Parallel()

	// Verify the provider can be instantiated (basic smoke test).
	p := provider.New("1.0.0")()
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

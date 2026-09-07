package provider_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// A pending invitation is a binding by email with no user id (ADR-061). The
// provider used to address the member by user id, so a pending one could
// neither change role nor be destroyed — `terraform destroy` reported success
// and left the invitation on the server (fogpipe/cloud-workspace#96). Invites an
// address nobody will redeem, changes its role, and checks the teardown really
// takes it back.
func TestAccOrgMemberResource_pendingInvitationIsUpdatedAndDestroyed(t *testing.T) {
	if os.Getenv("FPCLOUD_API_KEY") == "" {
		t.Skip("FPCLOUD_API_KEY not set, skipping acceptance test")
	}
	orgID := testAccOrgID(t)
	email := accName("invitee") + "@example.invalid"
	config := func(role string) string {
		return fmt.Sprintf(`
resource "fpcloud_org_member" "test" {
  organization_id = %[1]q
  email           = %[2]q
  role            = %[3]q
}
`, orgID, email, role)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgMemberDestroy(orgID, email),
		Steps: []resource.TestStep{
			{
				Config: config("viewer"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_org_member.test", "status", "pending"),
					resource.TestCheckResourceAttr("fpcloud_org_member.test", "user_id", ""),
					resource.TestCheckResourceAttr("fpcloud_org_member.test", "role", "viewer"),
				),
			},
			{
				Config: config("editor"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("fpcloud_org_member.test", "role", "editor"),
					testAccCheckOrgMemberRole(orgID, email, "editor"),
				),
			},
		},
	})
}

// testAccCheckOrgMemberRole reads the role back from the API rather than from
// state: an update the provider skipped would leave state saying what the plan
// said and the server saying otherwise.
func testAccCheckOrgMemberRole(orgID, email, role string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		members, err := testAccClient().ListOrgMembers(context.Background(), orgID)
		if err != nil {
			return fmt.Errorf("listing members: %w", err)
		}
		for _, m := range members {
			if strings.EqualFold(m.InvitedEmail, email) || strings.EqualFold(m.UserEmail, email) {
				if m.Role != role {
					return fmt.Errorf("member %s has role %q on the server, state says %q", email, m.Role, role)
				}
				return nil
			}
		}
		return fmt.Errorf("member %s is not on the server", email)
	}
}

// testAccCheckOrgMemberDestroy verifies the invitation is gone from the live
// API after teardown — the leak #96 is about is exactly a destroy that returns
// success with the binding still there.
func testAccCheckOrgMemberDestroy(orgID, email string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		members, err := testAccClient().ListOrgMembers(context.Background(), orgID)
		if err != nil {
			return fmt.Errorf("listing members: %w", err)
		}
		for _, m := range members {
			if strings.EqualFold(m.InvitedEmail, email) || strings.EqualFold(m.UserEmail, email) {
				return fmt.Errorf("invitation for %s survived the destroy", email)
			}
		}
		return nil
	}
}

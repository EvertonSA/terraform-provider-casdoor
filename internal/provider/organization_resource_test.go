// Copyright (c) HashiCorp, Inc.

package provider

import (
	"fmt"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestCasdoorSDKClient tests the Casdoor SDK client directly to verify connectivity.
func TestCasdoorSDKClient(t *testing.T) {
	config := setupTestConfig(t)

	client := casdoorsdk.NewClient(
		config.Endpoint,
		config.ClientID,
		config.ClientSecret,
		config.Certificate,
		config.OrganizationName,
		config.ApplicationName,
	)

	// Test getting existing organizations
	orgs, err := client.GetOrganizations()
	if err != nil {
		t.Fatalf("Failed to get organizations: %v", err)
	}

	t.Logf("Found %d organizations", len(orgs))
	for _, org := range orgs {
		t.Logf("  - %s/%s (%s)", org.Owner, org.Name, org.DisplayName)
	}

	// Test CRUD operations with a test organization
	rName := "tf-test-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	// Create
	newOrg := &casdoorsdk.Organization{
		Owner:        "admin",
		Name:         rName,
		DisplayName:  "Test Organization",
		PasswordType: "bcrypt",
	}

	success, err := client.AddOrganization(newOrg)
	if err != nil {
		t.Fatalf("Failed to create organization: %v", err)
	}
	if !success {
		t.Fatal("AddOrganization returned false")
	}
	t.Logf("Created organization: %s", rName)

	// Read
	org, err := client.GetOrganization(rName)
	if err != nil {
		t.Fatalf("Failed to get organization: %v", err)
	}
	if org == nil {
		t.Fatal("GetOrganization returned nil")
	}
	t.Logf("Read organization: %s/%s", org.Owner, org.Name)

	// Update
	org.DisplayName = "Updated Test Organization"
	success, err = client.UpdateOrganization(org)
	if err != nil {
		t.Fatalf("Failed to update organization: %v", err)
	}
	if !success {
		t.Fatal("UpdateOrganization returned false")
	}
	t.Logf("Updated organization: %s", rName)

	// Verify update
	org, err = client.GetOrganization(rName)
	if err != nil {
		t.Fatalf("Failed to get updated organization: %v", err)
	}
	if org.DisplayName != "Updated Test Organization" {
		t.Fatalf("Expected DisplayName 'Updated Test Organization', got '%s'", org.DisplayName)
	}

	// Delete
	success, err = client.DeleteOrganization(&casdoorsdk.Organization{Owner: "admin", Name: rName})
	if err != nil {
		t.Fatalf("Failed to delete organization: %v", err)
	}
	if !success {
		t.Fatal("DeleteOrganization returned false")
	}
	t.Logf("Deleted organization: %s", rName)

	// Verify deletion
	org, err = client.GetOrganization(rName)
	if err != nil {
		t.Logf("GetOrganization after delete returned error (expected): %v", err)
	}
	if org != nil {
		t.Fatal("GetOrganization returned non-nil after deletion")
	}
}

func TestAccOrganizationResource_basic(t *testing.T) {
	config := setupTestConfig(t)
	rName := "tf-test-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resourceName := "casdoor_organization.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(config),
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccProviderConfig(config) + testAccOrganizationResourceConfig(rName, "Test Organization"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "display_name", "Test Organization"),
					resource.TestCheckResourceAttr(resourceName, "owner", "admin"),
					resource.TestCheckResourceAttr(resourceName, "password_type", "bcrypt"),
				),
			},
			// Update and Read testing
			{
				Config: testAccProviderConfig(config) + testAccOrganizationResourceConfig(rName, "Updated Organization"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "display_name", "Updated Organization"),
				),
			},
		},
	})
}

func TestAccOrganizationResource_import(t *testing.T) {
	config := setupTestConfig(t)
	rName := "tf-test-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resourceName := "casdoor_organization.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(config),
		Steps: []resource.TestStep{
			// Create the resource first
			{
				Config: testAccProviderConfig(config) + testAccOrganizationResourceConfig(rName, "Test Organization"),
			},
			// ImportState testing - use explicit ImportStateId
			{
				Config:                  testAccProviderConfig(config) + testAccOrganizationResourceConfig(rName, "Test Organization"),
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateId:           "admin/" + rName,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"master_password", "default_password", "master_verification_code"},
			},
		},
	})
}

func testAccOrganizationResourceConfig(name, displayName string) string {
	return fmt.Sprintf(`
resource "casdoor_organization" "test" {
  name         = %q
  display_name = %q
}
`, name, displayName)
}

func TestAccOrganizationResource_v4Attributes(t *testing.T) {
	config := setupTestConfig(t)
	rName := "tf-test-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	resourceName := "casdoor_organization.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(config),
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig(config) + fmt.Sprintf(`
resource "casdoor_organization" "test" {
  name                    = %q
  display_name            = "Test Organization V4"
  password_history_count  = 5
  disable_console         = true
  enable_exclusive_signin = true
  max_sessions            = 3
  default_token_fields    = ["name", "email"]
  use_permanent_avatar    = true
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "password_history_count", "5"),
					resource.TestCheckResourceAttr(resourceName, "disable_console", "true"),
					resource.TestCheckResourceAttr(resourceName, "enable_exclusive_signin", "true"),
					resource.TestCheckResourceAttr(resourceName, "max_sessions", "3"),
					resource.TestCheckResourceAttr(resourceName, "default_token_fields.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "use_permanent_avatar", "true"),
				),
			},
			{
				Config: testAccProviderConfig(config) + fmt.Sprintf(`
resource "casdoor_organization" "test" {
  name                    = %q
  display_name            = "Test Organization V4 Updated"
  password_history_count  = 10
  disable_console         = false
  enable_exclusive_signin = false
  max_sessions            = 5
  default_token_fields    = ["name"]
  use_permanent_avatar    = false
}
`, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "display_name", "Test Organization V4 Updated"),
					resource.TestCheckResourceAttr(resourceName, "password_history_count", "10"),
					resource.TestCheckResourceAttr(resourceName, "disable_console", "false"),
					resource.TestCheckResourceAttr(resourceName, "enable_exclusive_signin", "false"),
					resource.TestCheckResourceAttr(resourceName, "max_sessions", "5"),
					resource.TestCheckResourceAttr(resourceName, "default_token_fields.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "use_permanent_avatar", "false"),
				),
			},
		},
	})
}

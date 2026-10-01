// Copyright (c) HashiCorp, Inc.

package provider

import (
	"context"
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ProviderClient holds the Casdoor SDK client and detected server version.
type ProviderClient struct {
	Client  *casdoorsdk.Client
	Version *version.Version
}

// casdoorV4Version is the minimum semver version for Casdoor 4.x.
var casdoorV4Version = version.Must(version.NewVersion("4.0.0"))

// IsV4OrGreater reports whether the Casdoor server version is 4.0.0 or higher.
func (c *ProviderClient) IsV4OrGreater() bool {
	if c == nil || c.Version == nil {
		return false
	}
	return c.Version.GreaterThanOrEqual(casdoorV4Version)
}

// getProviderClient extracts *casdoorsdk.Client and *version.Version from req.ProviderData.
func getProviderClient(providerData any, diags *diag.Diagnostics) (*casdoorsdk.Client, *version.Version, bool) {
	if providerData == nil {
		return nil, nil, false
	}

	switch c := providerData.(type) {
	case *ProviderClient:
		return c.Client, c.Version, true
	case *casdoorsdk.Client:
		return c, nil, true
	default:
		diags.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderClient or *casdoorsdk.Client, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, nil, false
	}
}

// sdkError checks the (bool, error) result from Casdoor SDK calls and adds
// appropriate diagnostics. Returns true if an error was recorded (caller
// should return early).
func sdkError(diags *diag.Diagnostics, ok bool, err error, msg string) bool {
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Error %s", msg),
			err.Error(),
		)
		return true
	}

	if !ok {
		diags.AddError(
			fmt.Sprintf("Error %s", msg),
			fmt.Sprintf("Casdoor returned failure when %s", msg),
		)
		return true
	}

	return false
}

// stringListToSDK extracts a []string from a types.List. Returns nil if the
// list is null or unknown.
func stringListToSDK(ctx context.Context, list types.List) ([]string, diag.Diagnostics) {
	if list.IsNull() || list.IsUnknown() {
		return nil, nil
	}

	var result []string
	diags := list.ElementsAs(ctx, &result, false)

	return result, diags
}

// stringListFromSDK converts a []string to a types.List. Returns
// types.ListNull if the slice is nil or empty.
func stringListFromSDK(ctx context.Context, slice []string) (types.List, diag.Diagnostics) {
	if len(slice) == 0 {
		return types.ListNull(types.StringType), nil
	}

	return types.ListValueFrom(ctx, types.StringType, slice)
}

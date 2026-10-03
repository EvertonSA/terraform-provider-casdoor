// Copyright (c) HashiCorp, Inc.

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ProviderClient holds the Casdoor SDK client and the configured server version.
type ProviderClient struct {
	Client  *casdoorsdk.Client
	Version *version.Version
}

// casdoorV4Version is the minimum semver version for Casdoor 4.x.
var casdoorV4Version = version.Must(version.NewVersion("4.0.0"))

// isV4 reports whether the Casdoor server version is 4.0.0 or higher.
func isV4(v *version.Version) bool {
	return v != nil && v.GreaterThanOrEqual(casdoorV4Version)
}

// IsV4OrGreater reports whether the Casdoor server version is 4.0.0 or higher.
func (c *ProviderClient) IsV4OrGreater() bool {
	return c != nil && isV4(c.Version)
}

// getProviderClient extracts the provider's *casdoorsdk.Client and server version
// from req.ProviderData.
func getProviderClient(providerData any, diags *diag.Diagnostics) (*casdoorsdk.Client, *version.Version, bool) {
	switch c := providerData.(type) {
	case *ProviderClient:
		return c.Client, c.Version, true
	case nil:
		return nil, nil, false
	default:
		diags.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderClient, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, nil, false
	}
}

// DoPost's isForm/isFile flags: the provider always posts JSON bodies.
const (
	doPostIsJSON = false
	doPostIsFile = false
)

// postObject posts payload to action for the object id and requires Casdoor to
// report the write as affected. The SDK fails with an error when the response
// status is not "ok" (returning a nil response), so err == nil implies a
// non-nil response with status "ok"; this contract is pinned by
// TestPostObject_sdkStatusContract.
func postObject(client *casdoorsdk.Client, action, id string, payload []byte) error {
	resp, err := client.DoPost(action, map[string]string{"id": id}, payload, doPostIsJSON, doPostIsFile)
	if err != nil {
		return fmt.Errorf("casdoor %s for %q failed: %w", action, id, err)
	}
	if resp.Data != "Affected" {
		// Casdoor answers status "ok" with data "Unaffected" for a refused
		// write or an object deleted out of band. The typed SDK methods
		// surface this as affected=false; mirror that here for v3/v4 parity.
		return fmt.Errorf("casdoor %s for %q reported no change (data=%v)", action, id, resp.Data)
	}
	return nil
}

// decodeJSONNumber decodes JSON into v, keeping numbers as json.Number instead
// of float64 so integers survive the marshal→map→marshal round trip.
func decodeJSONNumber(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// serverMergeRequest describes a read-merge-write update of a Casdoor object.
type serverMergeRequest struct {
	// GetAction fetches the current server object, e.g. "get-organization".
	GetAction string
	// UpdateAction posts the merged object, e.g. "update-organization".
	UpdateAction string
	// ID is the object id, e.g. "admin/my-org".
	ID string
	// PlanObj is the SDK struct built from the Terraform plan.
	PlanObj any
	// ExtraFields are JSON keys overlaid after the plan keys (Casdoor 4.x
	// attributes the SDK struct does not model).
	ExtraFields map[string]any
	// ManagedKeys lists the JSON keys Terraform manages. ONLY these are
	// overlaid onto the server object; every other key in the server response
	// is posted back verbatim.
	ManagedKeys []string
	// ProtectedKeys lists JSON keys Casdoor masks with the "***" sentinel when
	// set (credentials). An empty plan value for these keys is never overlaid,
	// so the server's mask is sent back and its Omit(...) protection keeps the
	// stored value.
	ProtectedKeys []string
}

// updateWithServerMerge fetches the current server object, overlays the keys
// Terraform manages plus ExtraFields, posts the result and returns the merged
// object that was posted.
//
// Guarantees and limits:
//   - Keys the schema does not manage are never written; they are sent back
//     exactly as the server returned them, so fields Terraform cannot model
//     (e.g. registrationAccessToken, clientCert, kerberosKeytab) survive an
//     apply.
//   - ProtectedKeys ("***"-masked credentials) keep the server mask when the
//     plan does not configure them.
//   - The read-merge-write is not atomic: a concurrent change within the
//     window is re-persisted (last write wins).
func updateWithServerMerge(client *casdoorsdk.Client, req serverMergeRequest) (map[string]any, error) {
	rawURL := client.GetUrl(req.GetAction, map[string]string{"id": req.ID})
	rawBytes, err := client.DoGetBytes(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch existing object for %q: %w", req.ID, err)
	}

	var serverObj map[string]any
	if len(rawBytes) > 0 {
		if err := decodeJSONNumber(rawBytes, &serverObj); err != nil {
			return nil, fmt.Errorf("failed to decode existing object for %q: %w", req.ID, err)
		}
	}
	if serverObj == nil {
		// DoGetBytes returns the literal "null" when the API answers
		// data: null, which unmarshals into a nil map — e.g. the object was
		// deleted out of band between plan and apply.
		return nil, fmt.Errorf("casdoor returned no object for %q via %s (deleted out of band?)", req.ID, req.GetAction)
	}

	planBytes, err := json.Marshal(req.PlanObj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal plan object: %w", err)
	}
	var planMap map[string]any
	if err := json.Unmarshal(planBytes, &planMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan map: %w", err)
	}

	protected := make(map[string]bool, len(req.ProtectedKeys))
	for _, k := range req.ProtectedKeys {
		protected[k] = true
	}
	for _, key := range req.ManagedKeys {
		v, ok := planMap[key]
		if !ok {
			continue // key not modeled by the SDK struct (e.g. "id") — server value stands
		}
		if protected[key] {
			if s, isStr := v.(string); isStr && s == "" {
				continue // keep the server's "***" mask so the server's Omit() fires
			}
		}
		serverObj[key] = v
	}
	for k, v := range req.ExtraFields {
		serverObj[k] = v
	}

	mergedBytes, err := json.Marshal(serverObj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged payload: %w", err)
	}

	if err := postObject(client, req.UpdateAction, req.ID, mergedBytes); err != nil {
		return nil, err
	}
	return serverObj, nil
}

// postWithExtraFields serializes planObj, overlays extraFields, and posts the
// payload to action (e.g. add-organization).
func postWithExtraFields(client *casdoorsdk.Client, action, id string, planObj any, extraFields map[string]any) error {
	planBytes, err := json.Marshal(planObj)
	if err != nil {
		return fmt.Errorf("failed to marshal plan object: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(planBytes, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal plan map: %w", err)
	}
	for k, v := range extraFields {
		payload[k] = v
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	return postObject(client, action, id, bodyBytes)
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

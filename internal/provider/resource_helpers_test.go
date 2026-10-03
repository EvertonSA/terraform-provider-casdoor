// Copyright (c) HashiCorp, Inc.

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func newMockClient(t *testing.T, handler http.HandlerFunc) *casdoorsdk.Client {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return casdoorsdk.NewClient(ts.URL, "id", "secret", "cert", "built-in", "app-built-in")
}

func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

func TestUpdateWithServerMerge_preservesUnmanagedKeys(t *testing.T) {
	var capturedPayload map[string]any

	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			// Server returns an object with unmanaged keys
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"msg":    "",
				"data": map[string]any{
					"displayName":             "Old Name",
					"registrationAccessToken": "rat-123",
					"kerberosKeytab":          "***",
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}
			if err := json.Unmarshal(body, &capturedPayload); err != nil {
				t.Fatalf("failed to unmarshal body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"msg":    "",
				"data":   "Affected",
			})
			return
		}
	})

	planObj := map[string]any{
		"displayName":             "New Name",
		"registrationAccessToken": "",
		"kerberosKeytab":          "",
	}

	merged, err := updateWithServerMerge(client, serverMergeRequest{
		GetAction:    "get-application",
		UpdateAction: "update-application",
		ID:           "admin/app",
		PlanObj:      planObj,
		ManagedKeys:  []string{"displayName"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPayload["displayName"] != "New Name" {
		t.Errorf("expected displayName 'New Name', got %v", capturedPayload["displayName"])
	}
	if capturedPayload["registrationAccessToken"] != "rat-123" {
		t.Errorf("expected unmanaged registrationAccessToken 'rat-123' preserved, got %v", capturedPayload["registrationAccessToken"])
	}
	if capturedPayload["kerberosKeytab"] != "***" {
		t.Errorf("expected unmanaged kerberosKeytab '***' preserved, got %v", capturedPayload["kerberosKeytab"])
	}
	if merged["registrationAccessToken"] != "rat-123" {
		t.Errorf("expected returned merged map to contain 'rat-123', got %v", merged["registrationAccessToken"])
	}
}

func TestUpdateWithServerMerge_protectedEmptyKeepsMask(t *testing.T) {
	t.Run("empty keeps mask", func(t *testing.T) {
		var capturedPayload map[string]any

		client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "ok",
					"msg":    "",
					"data": map[string]any{
						"displayName":    "Org",
						"masterPassword": "***",
					},
				})
				return
			}
			if r.Method == http.MethodPost {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedPayload)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "ok",
					"msg":    "",
					"data":   "Affected",
				})
				return
			}
		})

		planObj := map[string]any{
			"displayName":    "Org",
			"masterPassword": "",
		}

		_, err := updateWithServerMerge(client, serverMergeRequest{
			GetAction:     "get-organization",
			UpdateAction:  "update-organization",
			ID:            "admin/org",
			PlanObj:       planObj,
			ManagedKeys:   []string{"displayName", "masterPassword"},
			ProtectedKeys: []string{"masterPassword"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedPayload["masterPassword"] != "***" {
			t.Errorf("expected masterPassword '***' to be preserved, got %v", capturedPayload["masterPassword"])
		}
	})

	t.Run("new secret updates mask", func(t *testing.T) {
		var capturedPayload map[string]any

		client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "ok",
					"msg":    "",
					"data": map[string]any{
						"displayName":    "Org",
						"masterPassword": "***",
					},
				})
				return
			}
			if r.Method == http.MethodPost {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedPayload)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "ok",
					"msg":    "",
					"data":   "Affected",
				})
				return
			}
		})

		planObj := map[string]any{
			"displayName":    "Org",
			"masterPassword": "new-secret",
		}

		_, err := updateWithServerMerge(client, serverMergeRequest{
			GetAction:     "get-organization",
			UpdateAction:  "update-organization",
			ID:            "admin/org",
			PlanObj:       planObj,
			ManagedKeys:   []string{"displayName", "masterPassword"},
			ProtectedKeys: []string{"masterPassword"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedPayload["masterPassword"] != "new-secret" {
			t.Errorf("expected masterPassword 'new-secret', got %v", capturedPayload["masterPassword"])
		}
	})
}

func TestUpdateWithServerMerge_extraFieldsWin(t *testing.T) {
	var capturedPayload map[string]any

	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"msg":    "",
				"data": map[string]any{
					"maxSessions": 1,
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedPayload)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"msg":    "",
				"data":   "Affected",
			})
			return
		}
	})

	planObj := map[string]any{
		"maxSessions": 2,
	}

	_, err := updateWithServerMerge(client, serverMergeRequest{
		GetAction:    "get-organization",
		UpdateAction: "update-organization",
		ID:           "admin/org",
		PlanObj:      planObj,
		ManagedKeys:  []string{"maxSessions"},
		ExtraFields:  map[string]any{"maxSessions": json.Number("3")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In capturedPayload unmarshaled without UseNumber, numbers become float64
	val, ok := capturedPayload["maxSessions"]
	if !ok {
		t.Fatalf("expected maxSessions in capturedPayload")
	}
	if v, isFloat := val.(float64); isFloat {
		if v != 3 {
			t.Errorf("expected maxSessions == 3, got %v", v)
		}
	} else if v, isStr := val.(string); isStr {
		if v != "3" {
			t.Errorf("expected maxSessions == '3', got %v", v)
		}
	}
}

func TestUpdateWithServerMerge_nullData(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"msg":    "",
			"data":   nil,
		})
	})

	_, err := updateWithServerMerge(client, serverMergeRequest{
		GetAction:    "get-organization",
		UpdateAction: "update-organization",
		ID:           "admin/deleted-org",
		PlanObj:      map[string]any{"name": "deleted-org"},
		ManagedKeys:  []string{"name"},
	})
	if err == nil {
		t.Fatal("expected error for data: null, got nil")
	}
	if !strings.Contains(err.Error(), "deleted out of band") {
		t.Errorf("expected error to mention 'deleted out of band', got: %v", err)
	}
}

func TestUpdateWithServerMerge_unaffected(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"msg":    "",
				"data": map[string]any{
					"name": "test",
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"msg":    "",
				"data":   "Unaffected",
			})
			return
		}
	})

	_, err := updateWithServerMerge(client, serverMergeRequest{
		GetAction:    "get-organization",
		UpdateAction: "update-organization",
		ID:           "admin/test",
		PlanObj:      map[string]any{"name": "test"},
		ManagedKeys:  []string{"name"},
	})
	if err == nil {
		t.Fatal("expected error for data: Unaffected, got nil")
	}
	if !strings.Contains(err.Error(), "reported no change") {
		t.Errorf("expected error to mention 'reported no change', got: %v", err)
	}
}

func TestPostObject_errorContext(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"msg":    "boom",
		})
	})

	err := postObject(client, "update-organization", "admin/my-org", []byte("{}"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "update-organization") {
		t.Errorf("expected error to contain action 'update-organization', got: %s", errStr)
	}
	if !strings.Contains(errStr, "admin/my-org") {
		t.Errorf("expected error to contain id 'admin/my-org', got: %s", errStr)
	}
	if !strings.Contains(errStr, "boom") {
		t.Errorf("expected error to contain msg 'boom', got: %s", errStr)
	}
}

func TestPostWithExtraFields(t *testing.T) {
	t.Run("affected success", func(t *testing.T) {
		var captured map[string]any

		client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &captured)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"msg":    "",
				"data":   "Affected",
			})
		})

		err := postWithExtraFields(
			client,
			"add-organization",
			"admin/my-org",
			map[string]any{"name": "my-org"},
			map[string]any{"extra": "field1"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured["extra"] != "field1" {
			t.Errorf("expected extra field 'field1', got %v", captured["extra"])
		}
		if captured["name"] != "my-org" {
			t.Errorf("expected plan field 'my-org', got %v", captured["name"])
		}
	})

	t.Run("unaffected error", func(t *testing.T) {
		client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"msg":    "",
				"data":   "Unaffected",
			})
		})

		err := postWithExtraFields(
			client,
			"add-organization",
			"admin/my-org",
			map[string]any{"name": "my-org"},
			map[string]any{"extra": "field1"},
		)
		if err == nil {
			t.Fatal("expected error for Unaffected, got nil")
		}
		if !strings.Contains(err.Error(), "reported no change") {
			t.Errorf("expected error to mention 'reported no change', got: %v", err)
		}
	})
}

func TestPostObject_sdkStatusContract(t *testing.T) {
	client := newMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"msg":    "x",
		})
	})

	resp, err := client.DoPost("test-action", map[string]string{"id": "test-id"}, []byte("{}"), false, false)
	if resp != nil {
		t.Errorf("expected resp == nil when status is not ok, got %+v", resp)
	}
	if err == nil {
		t.Fatal("expected err != nil when status is not ok, got nil")
	}
	if !strings.Contains(err.Error(), "x") {
		t.Errorf("expected err to contain 'x', got %v", err)
	}
}

func TestPopulateV4FieldsFromMap(t *testing.T) {
	ctx := context.Background()

	t.Run("json.Number decodes correctly", func(t *testing.T) {
		model := &OrganizationResourceModel{}
		var diags diag.Diagnostics
		rawMap := map[string]any{
			"passwordHistoryCount":  json.Number("5"),
			"disableConsole":        true,
			"enableExclusiveSignin": true,
			"maxSessions":           json.Number("10"),
		}
		populateV4FieldsFromMap(ctx, rawMap, model, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		if model.PasswordHistoryCount.ValueInt64() != 5 {
			t.Errorf("expected PasswordHistoryCount 5, got %v", model.PasswordHistoryCount)
		}
		if !model.DisableConsole.ValueBool() {
			t.Errorf("expected DisableConsole true, got %v", model.DisableConsole)
		}
		if !model.EnableExclusiveSignin.ValueBool() {
			t.Errorf("expected EnableExclusiveSignin true, got %v", model.EnableExclusiveSignin)
		}
		if model.MaxSessions.ValueInt64() != 10 {
			t.Errorf("expected MaxSessions 10, got %v", model.MaxSessions)
		}
	})

	t.Run("absent keys leave model untouched", func(t *testing.T) {
		model := &OrganizationResourceModel{
			PasswordHistoryCount:  types.Int64Value(42),
			DisableConsole:        types.BoolValue(true),
			EnableExclusiveSignin: types.BoolValue(true),
			MaxSessions:           types.Int64Value(99),
		}
		var diags diag.Diagnostics
		rawMap := map[string]any{}
		populateV4FieldsFromMap(ctx, rawMap, model, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		if model.PasswordHistoryCount.ValueInt64() != 42 {
			t.Errorf("expected PasswordHistoryCount untouched (42), got %v", model.PasswordHistoryCount)
		}
		if !model.DisableConsole.ValueBool() {
			t.Errorf("expected DisableConsole untouched (true), got %v", model.DisableConsole)
		}
		if !model.EnableExclusiveSignin.ValueBool() {
			t.Errorf("expected EnableExclusiveSignin untouched (true), got %v", model.EnableExclusiveSignin)
		}
		if model.MaxSessions.ValueInt64() != 99 {
			t.Errorf("expected MaxSessions untouched (99), got %v", model.MaxSessions)
		}
	})

	t.Run("wrong type leaves model untouched", func(t *testing.T) {
		model := &OrganizationResourceModel{
			PasswordHistoryCount: types.Int64Value(42),
			DisableConsole:       types.BoolValue(false),
			MaxSessions:          types.Int64Value(99),
		}
		var diags diag.Diagnostics
		rawMap := map[string]any{
			"passwordHistoryCount": "not-a-number",
			"disableConsole":       "not-a-bool",
			"maxSessions":          "not-a-number",
		}
		populateV4FieldsFromMap(ctx, rawMap, model, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		if model.PasswordHistoryCount.ValueInt64() != 42 {
			t.Errorf("expected PasswordHistoryCount untouched (42), got %v", model.PasswordHistoryCount)
		}
		if model.DisableConsole.ValueBool() != false {
			t.Errorf("expected DisableConsole untouched (false), got %v", model.DisableConsole)
		}
		if model.MaxSessions.ValueInt64() != 99 {
			t.Errorf("expected MaxSessions untouched (99), got %v", model.MaxSessions)
		}
	})

	t.Run("defaultTokenFields []any converts to list", func(t *testing.T) {
		model := &OrganizationResourceModel{}
		var diags diag.Diagnostics
		rawMap := map[string]any{
			"defaultTokenFields": []any{"field1", "field2"},
		}
		populateV4FieldsFromMap(ctx, rawMap, model, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		if model.DefaultTokenFields.IsNull() {
			t.Fatal("expected DefaultTokenFields not null")
		}
		var elements []string
		_ = model.DefaultTokenFields.ElementsAs(ctx, &elements, false)
		if len(elements) != 2 || elements[0] != "field1" || elements[1] != "field2" {
			t.Errorf("expected ['field1', 'field2'], got %v", elements)
		}
	})

	t.Run("defaultTokenFields non-list leaves model untouched", func(t *testing.T) {
		initialList, _ := types.ListValueFrom(ctx, types.StringType, []string{"keep-me"})
		model := &OrganizationResourceModel{
			DefaultTokenFields: initialList,
		}
		var diags diag.Diagnostics
		rawMap := map[string]any{
			"defaultTokenFields": "not-a-list",
		}
		populateV4FieldsFromMap(ctx, rawMap, model, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		var elements []string
		_ = model.DefaultTokenFields.ElementsAs(ctx, &elements, false)
		if len(elements) != 1 || elements[0] != "keep-me" {
			t.Errorf("expected DefaultTokenFields untouched (['keep-me']), got %v", elements)
		}
	})
}

func TestManagedKeysMatchSchemaAttributes(t *testing.T) {
	ctx := context.Background()

	// OrganizationResource
	{
		orgResource := &OrganizationResource{}
		var resp resource.SchemaResponse
		orgResource.Schema(ctx, resource.SchemaRequest{}, &resp)

		managedSet := make(map[string]bool, len(organizationManagedKeys))
		for _, k := range organizationManagedKeys {
			managedSet[k] = true
		}

		schemaSet := make(map[string]bool, len(resp.Schema.Attributes))
		for attrName := range resp.Schema.Attributes {
			camel := snakeToCamel(attrName)
			schemaSet[camel] = true
			if !managedSet[camel] {
				t.Errorf("Organization schema attribute %q (camel: %q) missing from organizationManagedKeys", attrName, camel)
			}
		}
		for _, k := range organizationManagedKeys {
			if !schemaSet[k] {
				t.Errorf("organizationManagedKeys contains %q which is not in Organization schema", k)
			}
		}
	}

	// ApplicationResource
	{
		appResource := &ApplicationResource{}
		var resp resource.SchemaResponse
		appResource.Schema(ctx, resource.SchemaRequest{}, &resp)

		managedSet := make(map[string]bool, len(applicationManagedKeys))
		for _, k := range applicationManagedKeys {
			managedSet[k] = true
		}

		schemaSet := make(map[string]bool, len(resp.Schema.Attributes))
		for attrName := range resp.Schema.Attributes {
			camel := snakeToCamel(attrName)
			schemaSet[camel] = true
			if !managedSet[camel] {
				t.Errorf("Application schema attribute %q (camel: %q) missing from applicationManagedKeys", attrName, camel)
			}
		}
		for _, k := range applicationManagedKeys {
			if !schemaSet[k] {
				t.Errorf("applicationManagedKeys contains %q which is not in Application schema", k)
			}
		}
	}
}

func TestIsV4(t *testing.T) {
	if isV4(nil) {
		t.Error("isV4(nil) expected false, got true")
	}

	v2, _ := version.NewVersion("2.9.0")
	if isV4(v2) {
		t.Errorf("isV4(%s) expected false, got true", v2)
	}

	v3, _ := version.NewVersion("3.153.0")
	if isV4(v3) {
		t.Errorf("isV4(%s) expected false, got true", v3)
	}

	v4, _ := version.NewVersion("4.0.0")
	if !isV4(v4) {
		t.Errorf("isV4(%s) expected true, got false", v4)
	}

	v412, _ := version.NewVersion("4.12.0")
	if !isV4(v412) {
		t.Errorf("isV4(%s) expected true, got false", v412)
	}
}

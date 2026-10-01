// Copyright (c) HashiCorp, Inc.

package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProviderClient_IsV4OrGreater(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		{"v4.0.0", "4.0.0", true},
		{"v4.12.0", "4.12.0", true},
		{"v4.0.0 with prefix", "v4.0.0", true},
		{"v3.153.0", "3.153.0", false},
		{"v3.0.0", "3.0.0", false},
		{"v3.0.0 with prefix", "v3.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := version.NewVersion(tt.version)
			if err != nil {
				t.Fatalf("unexpected error parsing version %q: %v", tt.version, err)
			}
			pc := &ProviderClient{Version: v}
			if got := pc.IsV4OrGreater(); got != tt.expected {
				t.Errorf("IsV4OrGreater() for %q = %v, want %v", tt.version, got, tt.expected)
			}
		})
	}

	t.Run("nil version", func(t *testing.T) {
		var pc *ProviderClient
		if pc.IsV4OrGreater() {
			t.Error("expected false for nil ProviderClient")
		}
		pc2 := &ProviderClient{Version: nil}
		if pc2.IsV4OrGreater() {
			t.Error("expected false for nil Version")
		}
	})
}

func TestOrganizationResource_validateV4Attributes(t *testing.T) {
	v3, _ := version.NewVersion("3.153.0")
	v4, _ := version.NewVersion("4.12.0")

	tests := []struct {
		name      string
		version   *version.Version
		plan      OrganizationResourceModel
		expectErr bool
	}{
		{
			name:    "v4 with v4 attributes set",
			version: v4,
			plan: OrganizationResourceModel{
				PasswordHistoryCount: types.Int64Value(5),
				DisableConsole:       types.BoolValue(true),
			},
			expectErr: false,
		},
		{
			name:    "v3 with default zero attributes",
			version: v3,
			plan: OrganizationResourceModel{
				PasswordHistoryCount:  types.Int64Value(0),
				DisableConsole:        types.BoolValue(false),
				EnableExclusiveSignin: types.BoolValue(false),
				MaxSessions:           types.Int64Value(0),
				DefaultTokenFields:    types.ListNull(types.StringType),
			},
			expectErr: false,
		},
		{
			name:    "v3 with password_history_count set",
			version: v3,
			plan: OrganizationResourceModel{
				PasswordHistoryCount: types.Int64Value(3),
			},
			expectErr: true,
		},
		{
			name:    "v3 with disable_console set",
			version: v3,
			plan: OrganizationResourceModel{
				DisableConsole: types.BoolValue(true),
			},
			expectErr: true,
		},
		{
			name:    "v3 with enable_exclusive_signin set",
			version: v3,
			plan: OrganizationResourceModel{
				EnableExclusiveSignin: types.BoolValue(true),
			},
			expectErr: true,
		},
		{
			name:    "v3 with max_sessions set",
			version: v3,
			plan: OrganizationResourceModel{
				MaxSessions: types.Int64Value(10),
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &OrganizationResource{version: tt.version}
			var diags diag.Diagnostics
			r.validateV4Attributes(tt.plan, &diags)
			if diags.HasError() != tt.expectErr {
				t.Errorf("validateV4Attributes() error = %v (hasError: %v), want error = %v", diags, diags.HasError(), tt.expectErr)
			}
		})
	}
}

func TestApplicationResource_isV4(t *testing.T) {
	v3, _ := version.NewVersion("3.153.0")
	v4, _ := version.NewVersion("4.0.0")

	r3 := &ApplicationResource{version: v3}
	if r3.isV4() {
		t.Error("expected false for v3")
	}

	r4 := &ApplicationResource{version: v4}
	if !r4.isV4() {
		t.Error("expected true for v4")
	}

	rNil := &ApplicationResource{version: nil}
	if rNil.isV4() {
		t.Error("expected false for nil version")
	}
}

func TestFetchCasdoorVersion(t *testing.T) {
	tests := []struct {
		name        string
		response    string
		statusCode  int
		expectedVer string
		expectErr   bool
	}{
		{
			name:        "v4 wrapped object with commitId",
			response:    `{"status":"ok","data":{"commitId":"e24a907810860b91bb4c060bec7e90132438eef5","commitOffset":0,"version":"v4.12.0"}}`,
			statusCode:  200,
			expectedVer: "4.12.0",
		},
		{
			name:        "v4 wrapped object with version only",
			response:    `{"status":"ok","data":{"version":"4.12.0"}}`,
			statusCode:  200,
			expectedVer: "4.12.0",
		},
		{
			name:        "v3 wrapped string without v prefix",
			response:    `{"status":"ok","data":"3.153.0"}`,
			statusCode:  200,
			expectedVer: "3.153.0",
		},
		{
			name:        "v3 wrapped string with v prefix",
			response:    `{"status":"ok","data":"v3.0.0"}`,
			statusCode:  200,
			expectedVer: "3.0.0",
		},
		{
			name:       "api error response",
			response:   `{"status":"error","msg":"Unauthorized operation"}`,
			statusCode: 200,
			expectErr:  true,
		},
		{
			name:       "missing version field in data object",
			response:   `{"status":"ok","data":{"other":"field"}}`,
			statusCode: 200,
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client := casdoorsdk.NewClient(server.URL, "clientID", "clientSecret", "cert", "org", "app")
			v, err := fetchCasdoorVersion(client)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error, got nil (version: %s)", v)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != tt.expectedVer {
				t.Errorf("got version %q, want %q", v, tt.expectedVer)
			}
		})
	}
}

// Copyright (c) HashiCorp, Inc.

package provider

import (
	"testing"

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

	tokenFieldsList, _ := types.ListValueFrom(t.Context(), types.StringType, []string{"field1"})

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
				DefaultTokenFields:   tokenFieldsList,
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
		{
			name:    "v3 with default_token_fields set",
			version: v3,
			plan: OrganizationResourceModel{
				DefaultTokenFields: tokenFieldsList,
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

func TestVersionParsingAndValidation(t *testing.T) {
	minimumVersion, _ := version.NewVersion("3.0.0")

	tests := []struct {
		name          string
		inputVersion  string
		expectParseOk bool
		isSupported   bool
	}{
		{"valid 4.12.0", "4.12.0", true, true},
		{"valid 3.153.0", "3.153.0", true, true},
		{"valid minimum 3.0.0", "3.0.0", true, true},
		{"valid with v prefix", "v4.0.0", true, true},
		{"unsupported old 2.9.0", "2.9.0", true, false},
		{"unsupported old 1.0.0", "1.0.0", true, false},
		{"invalid format", "not-a-version", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := version.NewVersion(tt.inputVersion)
			if tt.expectParseOk && err != nil {
				t.Fatalf("unexpected error parsing %q: %v", tt.inputVersion, err)
			}
			if !tt.expectParseOk {
				if err == nil {
					t.Fatalf("expected error parsing %q, got nil", tt.inputVersion)
				}
				return
			}

			supported := !v.LessThan(minimumVersion)
			if supported != tt.isSupported {
				t.Errorf("version support for %q = %v, want %v", tt.inputVersion, supported, tt.isSupported)
			}
		})
	}
}

func TestResolveInstalledVersion(t *testing.T) {
	t.Run("explicit in config takes precedence", func(t *testing.T) {
		t.Setenv("CASDOOR_INSTALLED_VERSION", "3.153.0")
		got, err := resolveInstalledVersion(types.StringValue("4.12.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "4.12.0" {
			t.Errorf("got %q, want 4.12.0", got)
		}
	})

	t.Run("fallback to CASDOOR_INSTALLED_VERSION", func(t *testing.T) {
		t.Setenv("CASDOOR_INSTALLED_VERSION", "4.12.0")
		got, err := resolveInstalledVersion(types.StringNull())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "4.12.0" {
			t.Errorf("got %q, want 4.12.0", got)
		}
	})

	t.Run("unknown config version returns error", func(t *testing.T) {
		t.Setenv("CASDOOR_INSTALLED_VERSION", "4.12.0")
		_, err := resolveInstalledVersion(types.StringUnknown())
		if err == nil {
			t.Fatal("expected error when config version is unknown, got nil")
		}
	})

	t.Run("missing both config and env returns error", func(t *testing.T) {
		t.Setenv("CASDOOR_INSTALLED_VERSION", "")
		t.Setenv("INSTALLED_VERSION", "4.12.0") // Other env vars must NOT be used
		t.Setenv("CASDOOR_VERSION", "4.12.0")   // Other env vars must NOT be used
		_, err := resolveInstalledVersion(types.StringNull())
		if err == nil {
			t.Fatal("expected error when both config and CASDOOR_INSTALLED_VERSION are missing, got nil")
		}
	})
}

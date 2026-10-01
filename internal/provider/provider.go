// Copyright (c) HashiCorp, Inc.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &CasdoorProvider{}

type CasdoorProvider struct {
	version string
}

type CasdoorProviderModel struct {
	Endpoint         types.String `tfsdk:"endpoint"`
	ClientID         types.String `tfsdk:"client_id"`
	ClientSecret     types.String `tfsdk:"client_secret"`
	Certificate      types.String `tfsdk:"certificate"`
	OrganizationName types.String `tfsdk:"organization_name"`
	ApplicationName  types.String `tfsdk:"application_name"`
	InstalledVersion types.String `tfsdk:"installed_version"`
	// Alternative auth: admin login.
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CasdoorProvider{
			version: version,
		}
	}
}

func (p *CasdoorProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "casdoor"
	resp.Version = p.version
}

func (p *CasdoorProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `The Casdoor provider allows you to manage Casdoor resources.

## Compatibility

| Provider | Casdoor server | Tested against |
| --- | --- | --- |
| >= v0.2.0 | >= 3.0.0, 4.x | 3.0.0, 3.153.0, and 4.12.0 |
| >= v0.1.0 | >= 3.0.0 | 3.0.0 and 3.153.0 |
| <= v0.0.13 | pre-3.x schema | — |

The provider supports both Casdoor 3.x and Casdoor 4.x. Configure ` + "`installed_version`" + ` (or set the ` + "`CASDOOR_INSTALLED_VERSION`" + ` environment variable) to declare the target server version.

Casdoor 3.0.0 changed several object schemas. This provider targets 3.x and 4.x:
attributes that earlier releases exposed but that no longer exist upstream —
such as ` + "`casdoor_adapter.table_name_prefix`" + ` and ` + "`casdoor_user.access_key`" + ` —
have been removed.

The acceptance suite runs against supported server versions (including 3.x and 4.x).

## Authentication

The provider supports two authentication methods:

### 1. OAuth Application Credentials (recommended for production)

` + "```hcl" + `
provider "casdoor" {
  endpoint          = "https://casdoor.example.com"
  client_id         = "your-client-id"
  client_secret     = "your-client-secret"
  certificate       = file("path/to/cert.pem")
  organization_name = "built-in"
  application_name  = "app-built-in"
  installed_version = "4.12.0"
}
` + "```" + `

### 2. Admin Username/Password (convenient for development)

` + "```hcl" + `
provider "casdoor" {
  endpoint          = "https://casdoor.example.com"
  organization_name = "built-in"
  application_name  = "app-built-in"
  username          = "admin"
  password          = var.casdoor_admin_password
  installed_version = "4.12.0"
}
` + "```" + `

When using username/password, the provider will login and automatically fetch the application's OAuth credentials.

## Obtaining OAuth Credentials

Casdoor generates random ` + "`clientId`" + `, ` + "`clientSecret`" + `, and a JWT certificate for its built-in application on first start.
To use known credentials from day one, use Casdoor's
[Data Initialization](https://casdoor.org/docs/deployment/data-initialization/) mechanism:

1. Generate your own ` + "`client_id`" + `, ` + "`client_secret`" + `, and certificate
2. Place them in an ` + "`init_data.json`" + ` file
3. Set ` + "`initDataNewOnly = false`" + ` and ` + "`initDataFile`" + ` in Casdoor's ` + "`app.conf`" + `
4. Use the same values in your provider configuration

See the [Casdoor Data Initialization docs](https://casdoor.org/docs/deployment/data-initialization/) for the full reference.
`,
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Description: "The Casdoor server endpoint URL (e.g., https://casdoor.example.com).",
				Required:    true,
			},
			"client_id": schema.StringAttribute{
				Description: "The OAuth2 client ID for the Casdoor application. Required if username is not set.",
				Optional:    true,
			},
			"client_secret": schema.StringAttribute{
				Description: "The OAuth2 client secret for the Casdoor application. Required if username is not set.",
				Optional:    true,
				Sensitive:   true,
			},
			"certificate": schema.StringAttribute{
				Description: "The X.509 certificate (public key) for JWT verification. Required if username is not set.",
				Optional:    true,
			},
			"organization_name": schema.StringAttribute{
				Description: "The organization name in Casdoor.",
				Required:    true,
			},
			"application_name": schema.StringAttribute{
				Description: "The application name in Casdoor.",
				Required:    true,
			},
			"username": schema.StringAttribute{
				Description: "Admin username for authentication. If set, the provider will login and fetch OAuth credentials automatically.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Admin password for authentication. Required if username is set.",
				Optional:    true,
				Sensitive:   true,
			},
			"installed_version": schema.StringAttribute{
				Description: "The installed version of the Casdoor server (e.g. 4.12.0). Can also be set via the CASDOOR_INSTALLED_VERSION environment variable.",
				Optional:    true,
			},
		},
	}
}

func (p *CasdoorProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config CasdoorProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var clientID, clientSecret, certificate string

	// Determine authentication method.
	useLoginAuth := !config.Username.IsNull() && config.Username.ValueString() != ""

	if useLoginAuth {
		// Validate password is provided.
		if config.Password.IsNull() || config.Password.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Password",
				"Password is required when using username authentication.",
			)
			return
		}

		// Login and fetch credentials.
		creds, err := fetchCredentialsViaLogin(
			config.Endpoint.ValueString(),
			config.OrganizationName.ValueString(),
			config.ApplicationName.ValueString(),
			config.Username.ValueString(),
			config.Password.ValueString(),
		)
		if err != nil {
			resp.Diagnostics.AddError(
				"Authentication Failed",
				fmt.Sprintf("Failed to authenticate with Casdoor: %s", err),
			)
			return
		}

		clientID = creds.ClientID
		clientSecret = creds.ClientSecret
		certificate = creds.Certificate
	} else {
		// Use OAuth credentials directly.
		if config.ClientID.IsNull() || config.ClientID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Client ID",
				"Either client_id or username must be provided for authentication.",
			)
			return
		}
		if config.ClientSecret.IsNull() || config.ClientSecret.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Client Secret",
				"client_secret is required when using OAuth authentication.",
			)
			return
		}
		if config.Certificate.IsNull() || config.Certificate.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Certificate",
				"certificate is required when using OAuth authentication.",
			)
			return
		}

		clientID = config.ClientID.ValueString()
		clientSecret = config.ClientSecret.ValueString()
		certificate = config.Certificate.ValueString()
	}

	client := casdoorsdk.NewClient(
		config.Endpoint.ValueString(),
		clientID,
		clientSecret,
		certificate,
		config.OrganizationName.ValueString(),
		config.ApplicationName.ValueString(),
	)

	installedVersion, err := resolveInstalledVersion(config.InstalledVersion)
	if err != nil {
		resp.Diagnostics.AddError("Missing Required Casdoor Version", err.Error())
		return
	}

	parsedVersion, err := version.NewVersion(installedVersion)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Casdoor Version",
			fmt.Sprintf("Failed to parse Casdoor version %q: %s", installedVersion, err),
		)
		return
	}

	minimumVersion, _ := version.NewVersion("3.0.0")
	if parsedVersion.LessThan(minimumVersion) {
		resp.Diagnostics.AddError(
			"Unsupported Casdoor Version",
			fmt.Sprintf("Unsupported version of Casdoor. Minimum supported version is %s. Running version is %s.", minimumVersion, installedVersion),
		)
		return
	}

	providerClient := &ProviderClient{
		Client:  client,
		Version: parsedVersion,
	}

	resp.DataSourceData = providerClient
	resp.ResourceData = providerClient
}

// resolveInstalledVersion returns the explicit version from config or CASDOOR_INSTALLED_VERSION env var.
func resolveInstalledVersion(configVersion types.String) (string, error) {
	v := configVersion.ValueString()
	if v == "" {
		v = os.Getenv("CASDOOR_INSTALLED_VERSION")
	}
	if v == "" {
		return "", fmt.Errorf("the Casdoor server version must be explicitly configured via the 'installed_version' attribute or the CASDOOR_INSTALLED_VERSION environment variable")
	}
	return v, nil
}

// appCredentials holds the OAuth credentials fetched via login.
type appCredentials struct {
	ClientID     string
	ClientSecret string
	Certificate  string
}

// fetchCredentialsViaLogin authenticates with username/password and fetches application credentials.
func fetchCredentialsViaLogin(endpoint, organization, application, username, password string) (*appCredentials, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	client := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}

	// Step 1: Login.
	loginPayload := map[string]string{
		"application":  application,
		"organization": organization,
		"username":     username,
		"password":     password,
		"type":         "login",
	}
	loginBody, _ := json.Marshal(loginPayload)

	loginResp, err := client.Post(
		endpoint+"/api/login",
		"application/json",
		strings.NewReader(string(loginBody)),
	)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}
	defer func() { _ = loginResp.Body.Close() }()

	var loginResult struct {
		Status string `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&loginResult); err != nil {
		return nil, fmt.Errorf("failed to decode login response: %w", err)
	}
	if loginResult.Status != "ok" {
		return nil, fmt.Errorf("login failed: %s", loginResult.Msg)
	}

	// Step 2: Get application details.
	appResp, err := client.Get(fmt.Sprintf("%s/api/get-application?id=admin/%s", endpoint, application))
	if err != nil {
		return nil, fmt.Errorf("get application request failed: %w", err)
	}
	defer func() { _ = appResp.Body.Close() }()

	var appResult struct {
		Status string `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
			Cert         string `json:"cert"`
		} `json:"data"`
	}
	if err := json.NewDecoder(appResp.Body).Decode(&appResult); err != nil {
		return nil, fmt.Errorf("failed to decode application response: %w", err)
	}

	// Step 3: Get certificate.
	certName := appResult.Data.Cert
	if certName == "" {
		certName = "cert-built-in"
	}

	certResp, err := client.Get(fmt.Sprintf("%s/api/get-cert?id=admin/%s", endpoint, certName))
	if err != nil {
		return nil, fmt.Errorf("get cert request failed: %w", err)
	}
	defer func() { _ = certResp.Body.Close() }()

	var certResult struct {
		Status string `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			Certificate string `json:"certificate"`
		} `json:"data"`
	}
	if err := json.NewDecoder(certResp.Body).Decode(&certResult); err != nil {
		return nil, fmt.Errorf("failed to decode cert response: %w", err)
	}

	certificate := certResult.Data.Certificate
	if certificate == "" {
		return nil, fmt.Errorf("certificate not found for application %s", application)
	}

	return &appCredentials{
		ClientID:     appResult.Data.ClientID,
		ClientSecret: appResult.Data.ClientSecret,
		Certificate:  certificate,
	}, nil
}

func (p *CasdoorProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAdapterResource,
		NewApplicationResource,
		NewCertResource,
		NewEnforcerResource,
		NewGroupResource,
		NewIdpResource,
		NewLdapResource,
		NewModelResource,
		NewOrganizationResource,
		NewPermissionResource,
		NewPlanResource,
		NewPricingResource,
		NewProductResource,
		NewResourceResource,
		NewRoleResource,
		NewSyncerResource,
		NewTokenResource,
		NewUserResource,
		NewWebhookResource,
	}
}

func (p *CasdoorProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

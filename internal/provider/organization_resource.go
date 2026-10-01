// Copyright (c) HashiCorp, Inc.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                = &OrganizationResource{}
	_ resource.ResourceWithConfigure   = &OrganizationResource{}
	_ resource.ResourceWithImportState = &OrganizationResource{}
)

type OrganizationResource struct {
	client  *casdoorsdk.Client
	version *version.Version
}

func (r *OrganizationResource) isV4() bool {
	return r.version != nil && r.version.GreaterThanOrEqual(casdoorV4Version)
}

// AccountItemModel represents an account item configuration.
type AccountItemModel struct {
	Name       types.String `tfsdk:"name"`
	Visible    types.Bool   `tfsdk:"visible"`
	ViewRule   types.String `tfsdk:"view_rule"`
	ModifyRule types.String `tfsdk:"modify_rule"`
	Regex      types.String `tfsdk:"regex"`
	Tab        types.String `tfsdk:"tab"`
}

// MfaItemModel represents an MFA item configuration.
type MfaItemModel struct {
	Name types.String `tfsdk:"name"`
	Rule types.String `tfsdk:"rule"`
}

// ThemeDataModel represents the theme configuration.
type ThemeDataModel struct {
	ThemeType    types.String `tfsdk:"theme_type"`
	ColorPrimary types.String `tfsdk:"color_primary"`
	BorderRadius types.Int64  `tfsdk:"border_radius"`
	IsCompact    types.Bool   `tfsdk:"is_compact"`
	IsEnabled    types.Bool   `tfsdk:"is_enabled"`
}

// AccountItemAttrTypes returns the attribute types for AccountItemModel.
func AccountItemAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":        types.StringType,
		"visible":     types.BoolType,
		"view_rule":   types.StringType,
		"modify_rule": types.StringType,
		"regex":       types.StringType,
		"tab":         types.StringType,
	}
}

// accountItemsFromSDK converts SDK account items into a Terraform list. Casdoor
// populates a default set on create, so this value always comes from the server.
func accountItemsFromSDK(items []*casdoorsdk.AccountItem) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	objType := types.ObjectType{AttrTypes: AccountItemAttrTypes()}
	if len(items) == 0 {
		return types.ListNull(objType), diags
	}

	objList := make([]attr.Value, 0, len(items))
	for _, a := range items {
		obj, d := types.ObjectValue(AccountItemAttrTypes(), map[string]attr.Value{
			"name":        types.StringValue(a.Name),
			"visible":     types.BoolValue(a.Visible),
			"view_rule":   types.StringValue(a.ViewRule),
			"modify_rule": types.StringValue(a.ModifyRule),
			"regex":       types.StringValue(a.Regex),
			"tab":         types.StringValue(a.Tab),
		})
		diags.Append(d...)
		objList = append(objList, obj)
	}

	list, d := types.ListValue(objType, objList)
	diags.Append(d...)

	return list, diags
}

// MfaItemAttrTypes returns the attribute types for MfaItemModel.
func MfaItemAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name": types.StringType,
		"rule": types.StringType,
	}
}

// ThemeDataAttrTypes returns the attribute types for ThemeDataModel.
func ThemeDataAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"theme_type":    types.StringType,
		"color_primary": types.StringType,
		"border_radius": types.Int64Type,
		"is_compact":    types.BoolType,
		"is_enabled":    types.BoolType,
	}
}

type OrganizationResourceModel struct {
	ID                     types.String  `tfsdk:"id"`
	Owner                  types.String  `tfsdk:"owner"`
	Name                   types.String  `tfsdk:"name"`
	CreatedTime            types.String  `tfsdk:"created_time"`
	DisplayName            types.String  `tfsdk:"display_name"`
	WebsiteURL             types.String  `tfsdk:"website_url"`
	Logo                   types.String  `tfsdk:"logo"`
	LogoDark               types.String  `tfsdk:"logo_dark"`
	Favicon                types.String  `tfsdk:"favicon"`
	HasPrivilegeConsent    types.Bool    `tfsdk:"has_privilege_consent"`
	PasswordType           types.String  `tfsdk:"password_type"`
	PasswordSalt           types.String  `tfsdk:"password_salt"`
	PasswordOptions        types.List    `tfsdk:"password_options"`
	PasswordObfuscatorType types.String  `tfsdk:"password_obfuscator_type"`
	PasswordObfuscatorKey  types.String  `tfsdk:"password_obfuscator_key"`
	PasswordExpireDays     types.Int64   `tfsdk:"password_expire_days"`
	CountryCodes           types.List    `tfsdk:"country_codes"`
	DefaultAvatar          types.String  `tfsdk:"default_avatar"`
	DefaultApplication     types.String  `tfsdk:"default_application"`
	UserTypes              types.List    `tfsdk:"user_types"`
	Tags                   types.List    `tfsdk:"tags"`
	Languages              types.List    `tfsdk:"languages"`
	ThemeData              types.Object  `tfsdk:"theme_data"`
	MasterPassword         types.String  `tfsdk:"master_password"`
	DefaultPassword        types.String  `tfsdk:"default_password"`
	MasterVerificationCode types.String  `tfsdk:"master_verification_code"`
	IPWhitelist            types.String  `tfsdk:"ip_whitelist"`
	InitScore              types.Int64   `tfsdk:"init_score"`
	EnableSoftDeletion     types.Bool    `tfsdk:"enable_soft_deletion"`
	IsProfilePublic        types.Bool    `tfsdk:"is_profile_public"`
	UseEmailAsUsername     types.Bool    `tfsdk:"use_email_as_username"`
	EnableTour             types.Bool    `tfsdk:"enable_tour"`
	DisableSignin          types.Bool    `tfsdk:"disable_signin"`
	IPRestriction          types.String  `tfsdk:"ip_restriction"`
	NavItems               types.List    `tfsdk:"nav_items"`
	UserNavItems           types.List    `tfsdk:"user_nav_items"`
	WidgetItems            types.List    `tfsdk:"widget_items"`
	MfaItems               types.List    `tfsdk:"mfa_items"`
	MfaRememberInHours     types.Int64   `tfsdk:"mfa_remember_in_hours"`
	AccountItems           types.List    `tfsdk:"account_items"`
	OrgBalance             types.Float64 `tfsdk:"org_balance"`
	UserBalance            types.Float64 `tfsdk:"user_balance"`
	BalanceCredit          types.Float64 `tfsdk:"balance_credit"`
	BalanceCurrency        types.String  `tfsdk:"balance_currency"`
	AccountMenu            types.String  `tfsdk:"account_menu"`
	DcrPolicy              types.String  `tfsdk:"dcr_policy"`
	UsePermanentAvatar     types.Bool    `tfsdk:"use_permanent_avatar"`
	PasswordHistoryCount   types.Int64   `tfsdk:"password_history_count"`
	TokenRetentionDays     types.Int64   `tfsdk:"token_retention_days"`
	RecordRetentionDays    types.Int64   `tfsdk:"record_retention_days"`
	DefaultTokenFormat     types.String  `tfsdk:"default_token_format"`
	DefaultTokenFields     types.List    `tfsdk:"default_token_fields"`
	DisableConsole         types.Bool    `tfsdk:"disable_console"`
	EnableExclusiveSignin  types.Bool    `tfsdk:"enable_exclusive_signin"`
	MaxSessions            types.Int64   `tfsdk:"max_sessions"`
	LdapAttributes         types.List    `tfsdk:"ldap_attributes"`
}

func NewOrganizationResource() resource.Resource {
	return &OrganizationResource{}
}

func (r *OrganizationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (r *OrganizationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Casdoor organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the organization in the format 'owner/name'.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"owner": schema.StringAttribute{
				Description: "The owner of the organization. Defaults to 'admin'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("admin"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The unique name of the organization.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"created_time": schema.StringAttribute{
				Description: "The time when the organization was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "The display name of the organization.",
				Required:    true,
			},
			"website_url": schema.StringAttribute{
				Description: "The website URL of the organization.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"logo": schema.StringAttribute{
				Description: "The logo URL of the organization.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"logo_dark": schema.StringAttribute{
				Description: "The dark mode logo URL of the organization.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"favicon": schema.StringAttribute{
				Description: "The favicon URL of the organization.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"has_privilege_consent": schema.BoolAttribute{
				Description: "Whether the organization has privilege consent enabled.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"password_type": schema.StringAttribute{
				Description: "The password hashing algorithm. Valid values: plain, bcrypt, sha256-salt, md5-salt, etc.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("bcrypt"),
			},
			"password_salt": schema.StringAttribute{
				Description: "The salt used for password hashing.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"password_options": schema.ListAttribute{
				Description: "Password complexity options.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				ElementType: types.StringType,
			},
			"password_obfuscator_type": schema.StringAttribute{
				Description: "The password obfuscator type.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"password_obfuscator_key": schema.StringAttribute{
				Description: "The password obfuscator key.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Default:     stringdefault.StaticString(""),
			},
			"password_expire_days": schema.Int64Attribute{
				Description: "Number of days before password expires. 0 means no expiration.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"country_codes": schema.ListAttribute{
				Description: "List of allowed country codes.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				ElementType: types.StringType,
			},
			"default_avatar": schema.StringAttribute{
				Description: "The default avatar URL for users.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"default_application": schema.StringAttribute{
				Description: "The default application name for this organization.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"user_types": schema.ListAttribute{
				Description: "List of user types allowed in the organization.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"tags": schema.ListAttribute{
				Description: "Tags for the organization.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"languages": schema.ListAttribute{
				Description: "Supported languages for the organization.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				ElementType: types.StringType,
			},
			"theme_data": schema.SingleNestedAttribute{
				Description: "Theme configuration for the organization.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"theme_type": schema.StringAttribute{
						Description: "The theme type (e.g., 'default', 'dark').",
						Optional:    true,
					},
					"color_primary": schema.StringAttribute{
						Description: "The primary color in hex format.",
						Optional:    true,
					},
					"border_radius": schema.Int64Attribute{
						Description: "The border radius in pixels.",
						Optional:    true,
					},
					"is_compact": schema.BoolAttribute{
						Description: "Whether to use compact mode.",
						Optional:    true,
					},
					"is_enabled": schema.BoolAttribute{
						Description: "Whether the theme is enabled.",
						Optional:    true,
					},
				},
			},
			"master_password": schema.StringAttribute{
				Description: "The master password for the organization.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Default:     stringdefault.StaticString(""),
			},
			"default_password": schema.StringAttribute{
				Description: "The default password for new users.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Default:     stringdefault.StaticString(""),
			},
			"master_verification_code": schema.StringAttribute{
				Description: "The master verification code.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Default:     stringdefault.StaticString(""),
			},
			"ip_whitelist": schema.StringAttribute{
				Description: "IP whitelist for the organization.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"init_score": schema.Int64Attribute{
				Description: "Initial score for new users.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"enable_soft_deletion": schema.BoolAttribute{
				Description: "Whether soft deletion is enabled.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"is_profile_public": schema.BoolAttribute{
				Description: "Whether user profiles are public by default.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"use_email_as_username": schema.BoolAttribute{
				Description: "Whether to use email as username.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"enable_tour": schema.BoolAttribute{
				Description: "Whether the tour guide is enabled.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"disable_signin": schema.BoolAttribute{
				Description: "Whether sign-in is disabled for the organization.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"ip_restriction": schema.StringAttribute{
				Description: "IP restriction rules.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"nav_items": schema.ListAttribute{
				Description: "List of navigation items.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"user_nav_items": schema.ListAttribute{
				Description: "List of user navigation items.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"widget_items": schema.ListAttribute{
				Description: "List of widget items.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"mfa_items": schema.ListNestedAttribute{
				Description: "List of MFA configurations.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the MFA method.",
							Required:    true,
						},
						"rule": schema.StringAttribute{
							Description: "The rule for the MFA method.",
							Optional:    true,
						},
					},
				},
			},
			"mfa_remember_in_hours": schema.Int64Attribute{
				Description: "Number of hours to remember MFA authentication.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(12),
			},
			"account_items": schema.ListNestedAttribute{
				Description: "List of account item configurations that control user profile fields visibility and editability.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the account item (field name).",
							Required:    true,
						},
						"visible": schema.BoolAttribute{
							Description: "Whether this field is visible.",
							Optional:    true,
						},
						"view_rule": schema.StringAttribute{
							Description: "Rule for viewing this field.",
							Optional:    true,
						},
						"modify_rule": schema.StringAttribute{
							Description: "Rule for modifying this field.",
							Optional:    true,
						},
						"regex": schema.StringAttribute{
							Description: "Regex pattern for field validation.",
							Optional:    true,
						},
						"tab": schema.StringAttribute{
							Description: "Tab for this account item.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString(""),
						},
					},
				},
			},
			"org_balance": schema.Float64Attribute{
				Description: "The organization balance.",
				Optional:    true,
				Computed:    true,
				Default:     float64default.StaticFloat64(0),
			},
			"user_balance": schema.Float64Attribute{
				Description: "The user balance.",
				Optional:    true,
				Computed:    true,
				Default:     float64default.StaticFloat64(0),
			},
			"balance_credit": schema.Float64Attribute{
				Description: "The balance credit.",
				Optional:    true,
				Computed:    true,
				Default:     float64default.StaticFloat64(0),
			},
			"balance_currency": schema.StringAttribute{
				Description: "The balance currency.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"account_menu": schema.StringAttribute{
				Description: "The account menu configuration.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"dcr_policy": schema.StringAttribute{
				Description: "The dynamic client registration policy.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"use_permanent_avatar": schema.BoolAttribute{
				Description: "Whether to use permanent avatar.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"password_history_count": schema.Int64Attribute{
				Description: "Number of previous passwords to remember to prevent reuse.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"token_retention_days": schema.Int64Attribute{
				Description: "Token retention days.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"record_retention_days": schema.Int64Attribute{
				Description: "Record retention days.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"default_token_format": schema.StringAttribute{
				Description: "Default token format (e.g. JWT).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"default_token_fields": schema.ListAttribute{
				Description: "Default token fields.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				ElementType: types.StringType,
			},
			"disable_console": schema.BoolAttribute{
				Description: "Whether to disable console.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"enable_exclusive_signin": schema.BoolAttribute{
				Description: "Whether to enable exclusive signin.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"max_sessions": schema.Int64Attribute{
				Description: "Maximum allowed sessions.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"ldap_attributes": schema.ListAttribute{
				Description: "LDAP attributes.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				ElementType: types.StringType,
			},
		},
	}
}

func (r *OrganizationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, ver, ok := getProviderClient(req.ProviderData, &resp.Diagnostics)
	if !ok {
		return
	}

	r.client = client
	r.version = ver
}

func organizationPlanToSDK(ctx context.Context, plan OrganizationResourceModel, createdTime string) (*casdoorsdk.Organization, diag.Diagnostics) {
	var diags diag.Diagnostics

	passwordOptions, d := stringListToSDK(ctx, plan.PasswordOptions)
	diags.Append(d...)
	countryCodes, d := stringListToSDK(ctx, plan.CountryCodes)
	diags.Append(d...)
	userTypes, d := stringListToSDK(ctx, plan.UserTypes)
	diags.Append(d...)
	tags, d := stringListToSDK(ctx, plan.Tags)
	diags.Append(d...)
	languages, d := stringListToSDK(ctx, plan.Languages)
	diags.Append(d...)
	navItems, d := stringListToSDK(ctx, plan.NavItems)
	diags.Append(d...)
	userNavItems, d := stringListToSDK(ctx, plan.UserNavItems)
	diags.Append(d...)
	widgetItems, d := stringListToSDK(ctx, plan.WidgetItems)
	diags.Append(d...)
	ldapAttributes, d := stringListToSDK(ctx, plan.LdapAttributes)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	// Convert nested objects.
	var themeData *casdoorsdk.ThemeData
	if !plan.ThemeData.IsNull() {
		var themeModel ThemeDataModel
		diags.Append(plan.ThemeData.As(ctx, &themeModel, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			return nil, diags
		}
		themeData = &casdoorsdk.ThemeData{
			ThemeType:    themeModel.ThemeType.ValueString(),
			ColorPrimary: themeModel.ColorPrimary.ValueString(),
			BorderRadius: int(themeModel.BorderRadius.ValueInt64()),
			IsCompact:    themeModel.IsCompact.ValueBool(),
			IsEnabled:    themeModel.IsEnabled.ValueBool(),
		}
	}

	mfaItems := make([]*casdoorsdk.MfaItem, 0)
	if !plan.MfaItems.IsNull() {
		var mfaModels []MfaItemModel
		diags.Append(plan.MfaItems.ElementsAs(ctx, &mfaModels, false)...)
		if diags.HasError() {
			return nil, diags
		}
		for _, m := range mfaModels {
			mfaItems = append(mfaItems, &casdoorsdk.MfaItem{
				Name: m.Name.ValueString(),
				Rule: m.Rule.ValueString(),
			})
		}
	}

	accountItems := make([]*casdoorsdk.AccountItem, 0)
	if !plan.AccountItems.IsNull() && !plan.AccountItems.IsUnknown() {
		var accountModels []AccountItemModel
		diags.Append(plan.AccountItems.ElementsAs(ctx, &accountModels, false)...)
		if diags.HasError() {
			return nil, diags
		}
		for _, a := range accountModels {
			accountItems = append(accountItems, &casdoorsdk.AccountItem{
				Name:       a.Name.ValueString(),
				Visible:    a.Visible.ValueBool(),
				ViewRule:   a.ViewRule.ValueString(),
				ModifyRule: a.ModifyRule.ValueString(),
				Regex:      a.Regex.ValueString(),
				Tab:        a.Tab.ValueString(),
			})
		}
	}

	return &casdoorsdk.Organization{
		Owner:                  plan.Owner.ValueString(),
		Name:                   plan.Name.ValueString(),
		CreatedTime:            createdTime,
		DisplayName:            plan.DisplayName.ValueString(),
		WebsiteUrl:             plan.WebsiteURL.ValueString(),
		Logo:                   plan.Logo.ValueString(),
		LogoDark:               plan.LogoDark.ValueString(),
		Favicon:                plan.Favicon.ValueString(),
		HasPrivilegeConsent:    plan.HasPrivilegeConsent.ValueBool(),
		PasswordType:           plan.PasswordType.ValueString(),
		PasswordSalt:           plan.PasswordSalt.ValueString(),
		PasswordOptions:        passwordOptions,
		PasswordObfuscatorType: plan.PasswordObfuscatorType.ValueString(),
		PasswordObfuscatorKey:  plan.PasswordObfuscatorKey.ValueString(),
		PasswordExpireDays:     int(plan.PasswordExpireDays.ValueInt64()),
		CountryCodes:           countryCodes,
		DefaultAvatar:          plan.DefaultAvatar.ValueString(),
		DefaultApplication:     plan.DefaultApplication.ValueString(),
		UserTypes:              userTypes,
		Tags:                   tags,
		Languages:              languages,
		ThemeData:              themeData,
		MasterPassword:         plan.MasterPassword.ValueString(),
		DefaultPassword:        plan.DefaultPassword.ValueString(),
		MasterVerificationCode: plan.MasterVerificationCode.ValueString(),
		IpWhitelist:            plan.IPWhitelist.ValueString(),
		InitScore:              int(plan.InitScore.ValueInt64()),
		EnableSoftDeletion:     plan.EnableSoftDeletion.ValueBool(),
		IsProfilePublic:        plan.IsProfilePublic.ValueBool(),
		UseEmailAsUsername:     plan.UseEmailAsUsername.ValueBool(),
		EnableTour:             plan.EnableTour.ValueBool(),
		DisableSignin:          plan.DisableSignin.ValueBool(),
		IpRestriction:          plan.IPRestriction.ValueString(),
		NavItems:               navItems,
		UserNavItems:           userNavItems,
		WidgetItems:            widgetItems,
		MfaItems:               mfaItems,
		MfaRememberInHours:     int(plan.MfaRememberInHours.ValueInt64()),
		AccountItems:           accountItems,
		OrgBalance:             plan.OrgBalance.ValueFloat64(),
		UserBalance:            plan.UserBalance.ValueFloat64(),
		BalanceCredit:          plan.BalanceCredit.ValueFloat64(),
		BalanceCurrency:        plan.BalanceCurrency.ValueString(),
		AccountMenu:            plan.AccountMenu.ValueString(),
		DcrPolicy:              plan.DcrPolicy.ValueString(),
		UsePermanentAvatar:     plan.UsePermanentAvatar.ValueBool(),
		TokenRetentionDays:     int(plan.TokenRetentionDays.ValueInt64()),
		RecordRetentionDays:    int(plan.RecordRetentionDays.ValueInt64()),
		DefaultTokenFormat:     plan.DefaultTokenFormat.ValueString(),
		LdapAttributes:         ldapAttributes,
	}, diags
}

func (r *OrganizationResource) validateV4Attributes(plan OrganizationResourceModel, diags *diag.Diagnostics) {
	if r.isV4() {
		return
	}
	verStr := "pre-4.x"
	if r.version != nil {
		verStr = r.version.String()
	}
	checkField := func(name string, isSet bool) {
		if isSet {
			diags.AddError(
				"Unsupported Attribute for Casdoor Version",
				fmt.Sprintf("attribute %q requires Casdoor 4.0.0 or later (running %s)", name, verStr),
			)
		}
	}
	checkField("password_history_count", !plan.PasswordHistoryCount.IsNull() && plan.PasswordHistoryCount.ValueInt64() != 0)
	checkField("disable_console", !plan.DisableConsole.IsNull() && plan.DisableConsole.ValueBool())
	checkField("enable_exclusive_signin", !plan.EnableExclusiveSignin.IsNull() && plan.EnableExclusiveSignin.ValueBool())
	checkField("max_sessions", !plan.MaxSessions.IsNull() && plan.MaxSessions.ValueInt64() != 0)
	checkField("default_token_fields", !plan.DefaultTokenFields.IsNull() && len(plan.DefaultTokenFields.Elements()) > 0)
}

func (r *OrganizationResource) populateV4FieldsFromRaw(ctx context.Context, orgId string, model *OrganizationResourceModel, diags *diag.Diagnostics) {
	rawUrl := r.client.GetUrl("get-organization", map[string]string{"id": orgId})
	rawBytes, err := r.client.DoGetBytes(rawUrl)
	if err != nil || len(rawBytes) == 0 {
		return
	}

	var rawMap map[string]any
	if err := json.Unmarshal(rawBytes, &rawMap); err != nil {
		return
	}

	if v, ok := rawMap["passwordHistoryCount"].(float64); ok {
		model.PasswordHistoryCount = types.Int64Value(int64(v))
	} else {
		model.PasswordHistoryCount = types.Int64Value(0)
	}
	if v, ok := rawMap["disableConsole"].(bool); ok {
		model.DisableConsole = types.BoolValue(v)
	} else {
		model.DisableConsole = types.BoolValue(false)
	}
	if v, ok := rawMap["enableExclusiveSignin"].(bool); ok {
		model.EnableExclusiveSignin = types.BoolValue(v)
	} else {
		model.EnableExclusiveSignin = types.BoolValue(false)
	}
	if v, ok := rawMap["maxSessions"].(float64); ok {
		model.MaxSessions = types.Int64Value(int64(v))
	} else {
		model.MaxSessions = types.Int64Value(0)
	}
	if list, ok := rawMap["defaultTokenFields"].([]any); ok && len(list) > 0 {
		strList := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				strList = append(strList, s)
			}
		}
		var d diag.Diagnostics
		model.DefaultTokenFields, d = stringListFromSDK(ctx, strList)
		diags.Append(d...)
	} else {
		model.DefaultTokenFields = types.ListNull(types.StringType)
	}
}

func extractV4OrganizationFields(ctx context.Context, plan OrganizationResourceModel) map[string]any {
	extraFields := make(map[string]any)
	if !plan.PasswordHistoryCount.IsNull() && !plan.PasswordHistoryCount.IsUnknown() {
		extraFields["passwordHistoryCount"] = plan.PasswordHistoryCount.ValueInt64()
	}
	if !plan.DisableConsole.IsNull() && !plan.DisableConsole.IsUnknown() {
		extraFields["disableConsole"] = plan.DisableConsole.ValueBool()
	}
	if !plan.EnableExclusiveSignin.IsNull() && !plan.EnableExclusiveSignin.IsUnknown() {
		extraFields["enableExclusiveSignin"] = plan.EnableExclusiveSignin.ValueBool()
	}
	if !plan.MaxSessions.IsNull() && !plan.MaxSessions.IsUnknown() {
		extraFields["maxSessions"] = plan.MaxSessions.ValueInt64()
	}
	if !plan.DefaultTokenFields.IsNull() && !plan.DefaultTokenFields.IsUnknown() {
		var tokenFields []string
		_ = plan.DefaultTokenFields.ElementsAs(ctx, &tokenFields, false)
		extraFields["defaultTokenFields"] = tokenFields
	} else if plan.DefaultTokenFields.IsNull() {
		extraFields["defaultTokenFields"] = []string{}
	}
	return extraFields
}

func (r *OrganizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OrganizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.validateV4Attributes(plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createdTime := plan.CreatedTime.ValueString()
	if createdTime == "" {
		createdTime = time.Now().UTC().Format(time.RFC3339)
	}

	org, diags := organizationPlanToSDK(ctx, plan, createdTime)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgId := plan.Owner.ValueString() + "/" + plan.Name.ValueString()
	if r.isV4() {
		extraFields := extractV4OrganizationFields(ctx, plan)
		if err := postWithExtraFields(r.client, "add-organization", orgId, org, extraFields); err != nil {
			resp.Diagnostics.AddError("Error Creating Organization", fmt.Sprintf("Could not create organization %q: %s", plan.Name.ValueString(), err))
			return
		}
	} else {
		ok, err := r.client.AddOrganization(org)
		if sdkError(&resp.Diagnostics, ok, err, fmt.Sprintf("creating organization %q", plan.Name.ValueString())) {
			return
		}
	}

	// Read back the organization to get server-generated values like CreatedTime.
	createdOrg, err := r.client.GetOrganization(orgId)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Organization",
			fmt.Sprintf("Could not read organization %q after creation: %s", plan.Name.ValueString(), err),
		)
		return
	}

	// Populate from the server response, not the request: Casdoor fills in
	// defaults (account items, languages, country codes, ...) on create.
	if createdOrg != nil {
		plan.CreatedTime = types.StringValue(createdOrg.CreatedTime)
		plan.BalanceCurrency = types.StringValue(createdOrg.BalanceCurrency)

		plan.PasswordOptions, diags = stringListFromSDK(ctx, createdOrg.PasswordOptions)
		resp.Diagnostics.Append(diags...)
		plan.CountryCodes, diags = stringListFromSDK(ctx, createdOrg.CountryCodes)
		resp.Diagnostics.Append(diags...)
		plan.UserTypes, diags = stringListFromSDK(ctx, createdOrg.UserTypes)
		resp.Diagnostics.Append(diags...)
		plan.Tags, diags = stringListFromSDK(ctx, createdOrg.Tags)
		resp.Diagnostics.Append(diags...)
		plan.Languages, diags = stringListFromSDK(ctx, createdOrg.Languages)
		resp.Diagnostics.Append(diags...)
		plan.NavItems, diags = stringListFromSDK(ctx, createdOrg.NavItems)
		resp.Diagnostics.Append(diags...)
		plan.UserNavItems, diags = stringListFromSDK(ctx, createdOrg.UserNavItems)
		resp.Diagnostics.Append(diags...)
		plan.WidgetItems, diags = stringListFromSDK(ctx, createdOrg.WidgetItems)
		resp.Diagnostics.Append(diags...)
		plan.LdapAttributes, diags = stringListFromSDK(ctx, createdOrg.LdapAttributes)
		resp.Diagnostics.Append(diags...)
		if len(createdOrg.MfaItems) == 0 {
			plan.MfaItems = types.ListNull(types.ObjectType{AttrTypes: MfaItemAttrTypes()})
		}
		plan.AccountItems, diags = accountItemsFromSDK(createdOrg.AccountItems)
		resp.Diagnostics.Append(diags...)

		plan.UsePermanentAvatar = types.BoolValue(createdOrg.UsePermanentAvatar)
		plan.TokenRetentionDays = types.Int64Value(int64(createdOrg.TokenRetentionDays))
		plan.RecordRetentionDays = types.Int64Value(int64(createdOrg.RecordRetentionDays))
		plan.DefaultTokenFormat = types.StringValue(createdOrg.DefaultTokenFormat)
	}

	if r.isV4() {
		r.populateV4FieldsFromRaw(ctx, plan.Owner.ValueString()+"/"+plan.Name.ValueString(), &plan, &resp.Diagnostics)
	} else {
		plan.PasswordHistoryCount = types.Int64Value(0)
		plan.DisableConsole = types.BoolValue(false)
		plan.EnableExclusiveSignin = types.BoolValue(false)
		plan.MaxSessions = types.Int64Value(0)
		plan.DefaultTokenFields = types.ListNull(types.StringType)
	}

	plan.ID = types.StringValue(plan.Owner.ValueString() + "/" + plan.Name.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *OrganizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrganizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	org, err := r.client.GetOrganization(state.Owner.ValueString() + "/" + state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Organization",
			fmt.Sprintf("Could not read organization %q: %s", state.Name.ValueString(), err),
		)
		return
	}

	if org == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Set scalar fields.
	state.ID = types.StringValue(org.Owner + "/" + org.Name)
	state.Owner = types.StringValue(org.Owner)
	state.Name = types.StringValue(org.Name)
	state.CreatedTime = types.StringValue(org.CreatedTime)
	state.DisplayName = types.StringValue(org.DisplayName)
	state.WebsiteURL = types.StringValue(org.WebsiteUrl)
	state.Logo = types.StringValue(org.Logo)
	state.LogoDark = types.StringValue(org.LogoDark)
	state.Favicon = types.StringValue(org.Favicon)
	state.HasPrivilegeConsent = types.BoolValue(org.HasPrivilegeConsent)
	state.PasswordType = types.StringValue(org.PasswordType)
	state.PasswordSalt = types.StringValue(org.PasswordSalt)
	state.PasswordObfuscatorType = types.StringValue(org.PasswordObfuscatorType)
	state.PasswordObfuscatorKey = types.StringValue(org.PasswordObfuscatorKey)
	state.PasswordExpireDays = types.Int64Value(int64(org.PasswordExpireDays))
	state.DefaultAvatar = types.StringValue(org.DefaultAvatar)
	state.DefaultApplication = types.StringValue(org.DefaultApplication)
	// MasterPassword, DefaultPassword, MasterVerificationCode are always masked
	// by Casdoor API ("***"). Preserve real values from state; on import (when
	// state is null) fall back to empty string.
	if org.MasterPassword == "***" {
		if state.MasterPassword.IsNull() {
			state.MasterPassword = types.StringValue("")
		}
	} else {
		state.MasterPassword = types.StringValue(org.MasterPassword)
	}
	if org.DefaultPassword == "***" {
		if state.DefaultPassword.IsNull() {
			state.DefaultPassword = types.StringValue("")
		}
	} else {
		state.DefaultPassword = types.StringValue(org.DefaultPassword)
	}
	if org.MasterVerificationCode == "***" {
		if state.MasterVerificationCode.IsNull() {
			state.MasterVerificationCode = types.StringValue("")
		}
	} else {
		state.MasterVerificationCode = types.StringValue(org.MasterVerificationCode)
	}

	state.IPWhitelist = types.StringValue(org.IpWhitelist)
	state.InitScore = types.Int64Value(int64(org.InitScore))
	state.EnableSoftDeletion = types.BoolValue(org.EnableSoftDeletion)
	state.IsProfilePublic = types.BoolValue(org.IsProfilePublic)
	state.UseEmailAsUsername = types.BoolValue(org.UseEmailAsUsername)
	state.EnableTour = types.BoolValue(org.EnableTour)
	state.DisableSignin = types.BoolValue(org.DisableSignin)
	state.IPRestriction = types.StringValue(org.IpRestriction)
	state.MfaRememberInHours = types.Int64Value(int64(org.MfaRememberInHours))
	state.OrgBalance = types.Float64Value(org.OrgBalance)
	state.UserBalance = types.Float64Value(org.UserBalance)
	state.BalanceCredit = types.Float64Value(org.BalanceCredit)
	state.BalanceCurrency = types.StringValue(org.BalanceCurrency)
	state.AccountMenu = types.StringValue(org.AccountMenu)
	state.DcrPolicy = types.StringValue(org.DcrPolicy)
	state.UsePermanentAvatar = types.BoolValue(org.UsePermanentAvatar)
	state.TokenRetentionDays = types.Int64Value(int64(org.TokenRetentionDays))
	state.RecordRetentionDays = types.Int64Value(int64(org.RecordRetentionDays))
	state.DefaultTokenFormat = types.StringValue(org.DefaultTokenFormat)

	if r.isV4() {
		r.populateV4FieldsFromRaw(ctx, state.Owner.ValueString()+"/"+state.Name.ValueString(), &state, &resp.Diagnostics)
	} else {
		state.PasswordHistoryCount = types.Int64Value(0)
		state.DisableConsole = types.BoolValue(false)
		state.EnableExclusiveSignin = types.BoolValue(false)
		state.MaxSessions = types.Int64Value(0)
		state.DefaultTokenFields = types.ListNull(types.StringType)
	}

	// Convert string slices to list types.
	var diags diag.Diagnostics

	state.PasswordOptions, diags = stringListFromSDK(ctx, org.PasswordOptions)
	resp.Diagnostics.Append(diags...)
	state.CountryCodes, diags = stringListFromSDK(ctx, org.CountryCodes)
	resp.Diagnostics.Append(diags...)
	state.UserTypes, diags = stringListFromSDK(ctx, org.UserTypes)
	resp.Diagnostics.Append(diags...)
	state.Tags, diags = stringListFromSDK(ctx, org.Tags)
	resp.Diagnostics.Append(diags...)
	state.Languages, diags = stringListFromSDK(ctx, org.Languages)
	resp.Diagnostics.Append(diags...)
	state.NavItems, diags = stringListFromSDK(ctx, org.NavItems)
	resp.Diagnostics.Append(diags...)
	state.UserNavItems, diags = stringListFromSDK(ctx, org.UserNavItems)
	resp.Diagnostics.Append(diags...)
	state.WidgetItems, diags = stringListFromSDK(ctx, org.WidgetItems)
	resp.Diagnostics.Append(diags...)
	state.LdapAttributes, diags = stringListFromSDK(ctx, org.LdapAttributes)
	resp.Diagnostics.Append(diags...)

	// Convert ThemeData to object type.
	if org.ThemeData != nil {
		themeObj, diags := types.ObjectValue(ThemeDataAttrTypes(), map[string]attr.Value{
			"theme_type":    types.StringValue(org.ThemeData.ThemeType),
			"color_primary": types.StringValue(org.ThemeData.ColorPrimary),
			"border_radius": types.Int64Value(int64(org.ThemeData.BorderRadius)),
			"is_compact":    types.BoolValue(org.ThemeData.IsCompact),
			"is_enabled":    types.BoolValue(org.ThemeData.IsEnabled),
		})
		resp.Diagnostics.Append(diags...)
		state.ThemeData = themeObj
	} else {
		state.ThemeData = types.ObjectNull(ThemeDataAttrTypes())
	}

	// Convert MfaItems to list of objects.
	if len(org.MfaItems) > 0 {
		mfaObjList := make([]attr.Value, 0, len(org.MfaItems))
		for _, m := range org.MfaItems {
			mfaObj, diags := types.ObjectValue(MfaItemAttrTypes(), map[string]attr.Value{
				"name": types.StringValue(m.Name),
				"rule": types.StringValue(m.Rule),
			})
			resp.Diagnostics.Append(diags...)
			mfaObjList = append(mfaObjList, mfaObj)
		}
		mfaList, diags := types.ListValue(types.ObjectType{AttrTypes: MfaItemAttrTypes()}, mfaObjList)
		resp.Diagnostics.Append(diags...)
		state.MfaItems = mfaList
	} else {
		state.MfaItems = types.ListNull(types.ObjectType{AttrTypes: MfaItemAttrTypes()})
	}

	// Convert AccountItems to list of objects.
	state.AccountItems, diags = accountItemsFromSDK(org.AccountItems)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OrganizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OrganizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.validateV4Attributes(plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.isV4() {
		orgId := plan.Owner.ValueString() + "/" + plan.Name.ValueString()

		org, diags := organizationPlanToSDK(ctx, plan, plan.CreatedTime.ValueString())
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		extraFields := extractV4OrganizationFields(ctx, plan)
		if err := updateWithServerMerge(r.client, "get-organization", "update-organization", orgId, org, extraFields); err != nil {
			resp.Diagnostics.AddError("Error Updating Organization", fmt.Sprintf("Could not update organization %q: %s", plan.Name.ValueString(), err))
			return
		}

		updatedOrg, err := r.client.GetOrganization(orgId)
		if err != nil || updatedOrg == nil {
			resp.Diagnostics.AddError("Error Reading Organization After Update", fmt.Sprintf("Could not read organization %q: %v", plan.Name.ValueString(), err))
			return
		}

		plan.PasswordOptions, diags = stringListFromSDK(ctx, updatedOrg.PasswordOptions)
		resp.Diagnostics.Append(diags...)
		plan.CountryCodes, diags = stringListFromSDK(ctx, updatedOrg.CountryCodes)
		resp.Diagnostics.Append(diags...)
		plan.UserTypes, diags = stringListFromSDK(ctx, updatedOrg.UserTypes)
		resp.Diagnostics.Append(diags...)
		plan.Tags, diags = stringListFromSDK(ctx, updatedOrg.Tags)
		resp.Diagnostics.Append(diags...)
		plan.Languages, diags = stringListFromSDK(ctx, updatedOrg.Languages)
		resp.Diagnostics.Append(diags...)
		plan.NavItems, diags = stringListFromSDK(ctx, updatedOrg.NavItems)
		resp.Diagnostics.Append(diags...)
		plan.UserNavItems, diags = stringListFromSDK(ctx, updatedOrg.UserNavItems)
		resp.Diagnostics.Append(diags...)
		plan.WidgetItems, diags = stringListFromSDK(ctx, updatedOrg.WidgetItems)
		resp.Diagnostics.Append(diags...)
		plan.LdapAttributes, diags = stringListFromSDK(ctx, updatedOrg.LdapAttributes)
		resp.Diagnostics.Append(diags...)
		if len(updatedOrg.MfaItems) == 0 {
			plan.MfaItems = types.ListNull(types.ObjectType{AttrTypes: MfaItemAttrTypes()})
		}
		plan.AccountItems, diags = accountItemsFromSDK(updatedOrg.AccountItems)
		resp.Diagnostics.Append(diags...)

		plan.UsePermanentAvatar = types.BoolValue(updatedOrg.UsePermanentAvatar)
		plan.TokenRetentionDays = types.Int64Value(int64(updatedOrg.TokenRetentionDays))
		plan.RecordRetentionDays = types.Int64Value(int64(updatedOrg.RecordRetentionDays))
		plan.DefaultTokenFormat = types.StringValue(updatedOrg.DefaultTokenFormat)

		r.populateV4FieldsFromRaw(ctx, orgId, &plan, &resp.Diagnostics)
	} else {
		org, diags := organizationPlanToSDK(ctx, plan, plan.CreatedTime.ValueString())
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		ok, err := r.client.UpdateOrganization(org)
		if sdkError(&resp.Diagnostics, ok, err, fmt.Sprintf("updating organization %q", plan.Name.ValueString())) {
			return
		}

		// Set list values to null if empty to match plan.
		plan.PasswordOptions, diags = stringListFromSDK(ctx, org.PasswordOptions)
		resp.Diagnostics.Append(diags...)
		plan.CountryCodes, diags = stringListFromSDK(ctx, org.CountryCodes)
		resp.Diagnostics.Append(diags...)
		plan.UserTypes, diags = stringListFromSDK(ctx, org.UserTypes)
		resp.Diagnostics.Append(diags...)
		plan.Tags, diags = stringListFromSDK(ctx, org.Tags)
		resp.Diagnostics.Append(diags...)
		plan.Languages, diags = stringListFromSDK(ctx, org.Languages)
		resp.Diagnostics.Append(diags...)
		plan.NavItems, diags = stringListFromSDK(ctx, org.NavItems)
		resp.Diagnostics.Append(diags...)
		plan.UserNavItems, diags = stringListFromSDK(ctx, org.UserNavItems)
		resp.Diagnostics.Append(diags...)
		plan.WidgetItems, diags = stringListFromSDK(ctx, org.WidgetItems)
		resp.Diagnostics.Append(diags...)
		if len(org.MfaItems) == 0 {
			plan.MfaItems = types.ListNull(types.ObjectType{AttrTypes: MfaItemAttrTypes()})
		}
		plan.AccountItems, diags = accountItemsFromSDK(org.AccountItems)
		resp.Diagnostics.Append(diags...)
	}

	plan.ID = types.StringValue(plan.Owner.ValueString() + "/" + plan.Name.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *OrganizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OrganizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	org := &casdoorsdk.Organization{
		Owner: state.Owner.ValueString(),
		Name:  state.Name.ValueString(),
	}

	ok, err := r.client.DeleteOrganization(org)
	if sdkError(&resp.Diagnostics, ok, err, fmt.Sprintf("deleting organization %q", state.Name.ValueString())) {
		return
	}
}

func (r *OrganizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importStateOwnerName(ctx, req, resp)
}

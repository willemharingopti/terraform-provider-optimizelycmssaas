package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &cmsProvider{}

var pathKey = path.Root("key")

type cmsProvider struct{ version string }

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &cmsProvider{version: version} }
}

type providerModel struct {
	BaseURL      types.String `tfsdk:"base_url"`
	TokenURL     types.String `tfsdk:"token_url"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	APIVersion   types.String `tfsdk:"api_version"`
	RootKey      types.String `tfsdk:"content_root_key"`
}

func (p *cmsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "optimizelycmssaas"
	resp.Version = p.version
}

func (p *cmsProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Optimizely CMS (SaaS) via its REST API.",
		Attributes: map[string]schema.Attribute{
			"base_url":         schema.StringAttribute{Optional: true, Description: "API host, https only; the API version is added for you (a trailing /vN is accepted and used as the version unless api_version is set). Env: OPTIMIZELY_CMS_BASE_URL. Default " + client.DefaultHost},
			"content_root_key": schema.StringAttribute{Optional: true, Description: "Key of the root of the content tree, used by the optimizelycmssaas_content_root data source. Env: OPTIMIZELY_CMS_CONTENT_ROOT_KEY. Default " + client.WellKnownRootKey + ", the built-in root observed on every instance checked. A key with dashes is accepted. When set, that key is used or the lookup fails; it never falls back to another one."},
			"api_version":      schema.StringAttribute{Optional: true, Description: "REST API version, such as v1 or v2. Env: OPTIMIZELY_CMS_API_VERSION. Default " + client.DefaultAPIVersion + ", the version this provider is built and tested against: other versions may differ and are used at your own risk."},
			"token_url":        schema.StringAttribute{Optional: true, Description: "OAuth token URL (https only). Env: OPTIMIZELY_CMS_TOKEN_URL. Default <base_url>/oauth/token."},
			"client_id":        schema.StringAttribute{Optional: true, Description: "API client name, as shown when the API client was created. Env: OPTIMIZELY_CMS_CLIENT_ID."},
			"client_secret":    schema.StringAttribute{Optional: true, Sensitive: true, Description: "API client secret. Prefer env OPTIMIZELY_CMS_CLIENT_SECRET so it stays out of config/state."},
		},
	}
}

func firstNonEmpty(v types.String, env, def string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	if e := os.Getenv(env); e != "" {
		return e
	}
	return def
}

func (p *cmsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := client.New(
		firstNonEmpty(cfg.BaseURL, "OPTIMIZELY_CMS_BASE_URL", client.DefaultHost),
		firstNonEmpty(cfg.TokenURL, "OPTIMIZELY_CMS_TOKEN_URL", ""),
		firstNonEmpty(cfg.ClientID, "OPTIMIZELY_CMS_CLIENT_ID", ""),
		firstNonEmpty(cfg.ClientSecret, "OPTIMIZELY_CMS_CLIENT_SECRET", ""),
		client.WithAPIVersion(firstNonEmpty(cfg.APIVersion, "OPTIMIZELY_CMS_API_VERSION", "")),
		client.WithRootKey(firstNonEmpty(cfg.RootKey, "OPTIMIZELY_CMS_CONTENT_ROOT_KEY", "")),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider configuration", err.Error())
		return
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *cmsProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewContentTypeResource, NewContentSourceResource, NewContentTypeBindingResource, NewLocaleResource, NewDisplayTemplateResource, NewApplicationResource, NewPropertyGroupResource, NewBlueprintResource, NewContentResource,
	}
}

func (p *cmsProvider) DataSources(context.Context) []func() datasource.DataSource {
	ds := []func() datasource.DataSource{NewPropertyFormatDataSource, NewManifestDataSource, NewContentRootDataSource}
	for _, spec := range lookupSpecs() {
		spec := spec
		ds = append(ds, func() datasource.DataSource { return &lookupDataSource{spec: spec} })
	}
	return ds
}

// clientFrom extracts the configured client during resource Configure.
func clientFrom(data any, diags *diag.Diagnostics) *client.Client {
	if data == nil {
		return nil // provider not configured yet (e.g. during validation)
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("got %T", data))
	}
	return c
}

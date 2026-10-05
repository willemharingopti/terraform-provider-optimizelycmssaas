package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource                     = &contentRootDataSource{}
	_ datasource.DataSourceWithConfigure        = &contentRootDataSource{}
	_ datasource.DataSourceWithConfigValidators = &contentRootDataSource{}
)

func NewContentRootDataSource() datasource.DataSource { return &contentRootDataSource{} }

type contentRootDataSource struct{ c *client.Client }

type contentRootModel struct {
	FromApplication types.String `tfsdk:"from_application"`
	FromContent     types.String `tfsdk:"from_content"`
	Key             types.String `tfsdk:"key"`
	Reference       types.String `tfsdk:"reference"`
	FoundVia        types.String `tfsdk:"found_via"`
}

func (d *contentRootDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_content_root"
}

func (d *contentRootDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Finds the root of the content tree: the top-level container that holds the sites' start pages. " +
			"The API has no root endpoint. By default the root's key (the provider's content_root_key; the built-in root's fixed key " +
			"unless overridden) is checked first, which works on an empty instance. If it was left at the default and is not present, the " +
			"root is found by walking up from the start page of the first existing application this API client may read. An explicitly " +
			"configured key is never replaced by that fallback. Set from_application or from_content to force the walk-up.",
		Attributes: map[string]schema.Attribute{
			"from_application": schema.StringAttribute{Optional: true, Description: "Key of an application whose start page to walk up from."},
			"from_content":     schema.StringAttribute{Optional: true, Description: "Key of any readable content item to walk up from."},
			"key":              schema.StringAttribute{Computed: true, Description: "Key of the root. Use as `container` of a new site's start page."},
			"reference":        schema.StringAttribute{Computed: true, Description: "The root as a content reference, cms://content/<key>."},
			"found_via":        schema.StringAttribute{Computed: true, Description: "How the root was found, e.g. the application used."},
		},
	}
}

func (d *contentRootDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.Conflicting(path.MatchRoot("from_application"), path.MatchRoot("from_content")),
	}
}

func (d *contentRootDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *contentRootDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg contentRootModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var startKeys []string // candidates to walk up from, with how each was chosen
	var via []string
	switch {
	case !cfg.FromContent.IsNull():
		startKeys, via = []string{cfg.FromContent.ValueString()}, []string{"content " + cfg.FromContent.ValueString()}
	case !cfg.FromApplication.IsNull():
		app, err := d.c.Applications.Get(ctx, cfg.FromApplication.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading application failed", err.Error())
			return
		}
		k, ok := client.ContentKeyFromRef(app.EntryPoint)
		if !ok {
			resp.Diagnostics.AddError("Unusable entry point", fmt.Sprintf("application %q has entry point %q, which is not a content reference", app.Key, app.EntryPoint))
			return
		}
		startKeys, via = []string{k}, []string{"application " + app.Key}
	default:
		// Fast path: the root has a configured (by default fixed, built-in) key, so no existing
		// site is needed; it also works on an empty instance.
		rootKey, explicit := d.c.RootKey()
		if root, ok := d.c.ConfiguredRoot(ctx); ok {
			via := "well-known root key"
			if explicit {
				via = "configured root key"
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, contentRootModel{
				Key: types.StringValue(root), Reference: types.StringValue("cms://content/" + root),
				FoundVia: types.StringValue(via),
			})...)
			return
		}
		if explicit { // a deliberate choice must not silently turn into a different root
			resp.Diagnostics.AddError("Configured content root not usable",
				fmt.Sprintf("content_root_key / OPTIMIZELY_CMS_CONTENT_ROOT_KEY is set to %q, but that item does not exist, "+
					"cannot be read by this API client, or has a parent, so it is not a root. Fix the key, or unset it to "+
					"use the built-in default and the application-based lookup.", rootKey))
			return
		}
		apps, err := d.c.Applications.List(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing applications failed", err.Error())
			return
		}
		for _, a := range apps {
			if k, ok := client.ContentKeyFromRef(a.EntryPoint); ok {
				startKeys, via = append(startKeys, k), append(via, "application "+a.Key)
			}
		}
	}

	explicit := !cfg.FromContent.IsNull() || !cfg.FromApplication.IsNull()
	var skipped []string
	for i, start := range startKeys {
		root, _, err := d.c.FindRoot(ctx, start)
		if err != nil {
			if explicit {
				resp.Diagnostics.AddError("Finding the content root failed", err.Error())
				return
			}
			skipped = append(skipped, via[i]) // not readable by this API client; try the next one
			continue
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, contentRootModel{
			FromApplication: cfg.FromApplication, FromContent: cfg.FromContent,
			Key: types.StringValue(root), Reference: types.StringValue("cms://content/" + root), FoundVia: types.StringValue(via[i]),
		})...)
		return
	}
	detail := "No application with a start page this API client may read was found, so the root could not be located."
	if len(skipped) > 0 {
		detail += " Tried: " + strings.Join(skipped, ", ") + "."
	}
	resp.Diagnostics.AddError("Content root not found",
		detail+" Set from_content to the key of any content item the API client can read, or use a container key directly.")
}

package provider

import (
	"context"
	"fmt"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// lookupSpec describes a by-key data source mirroring a resource. Its schema is
// derived from the resource schema (everything computed, key required) so the
// two cannot drift apart.
type lookupSpec struct {
	name     string // type name suffix, e.g. "locale"
	resource func() resource.Resource
	fetch    func(ctx context.Context, c *client.Client, key string) (any, error)
}

func lookupSpecs() []lookupSpec {
	return []lookupSpec{
		{"content_type", NewContentTypeResource, func(ctx context.Context, c *client.Client, key string) (any, error) {
			v, err := c.ContentTypes.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return contentTypeFromAPI(v, contentTypeModel{}), nil
		}},
		{"content_source", NewContentSourceResource, func(ctx context.Context, c *client.Client, key string) (any, error) {
			v, err := c.ContentSources.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return contentSourceFromAPI(v), nil
		}},
		{"content_type_binding", NewContentTypeBindingResource, func(ctx context.Context, c *client.Client, key string) (any, error) {
			v, err := c.ContentTypeBindings.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return bindingFromAPI(v), nil
		}},
		{"locale", NewLocaleResource, func(ctx context.Context, c *client.Client, key string) (any, error) {
			v, err := c.Locales.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return localeFromAPI(v), nil
		}},
		{"display_template", NewDisplayTemplateResource, func(ctx context.Context, c *client.Client, key string) (any, error) {
			v, err := c.DisplayTemplates.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return displayTemplateFromAPI(v), nil
		}},
		{"application", NewApplicationResource, func(ctx context.Context, c *client.Client, key string) (any, error) {
			v, err := c.Applications.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return applicationFromAPI(v, types.StringNull()), nil
		}},
		{"property_group", NewPropertyGroupResource, func(ctx context.Context, c *client.Client, key string) (any, error) {
			v, err := c.PropertyGroups.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return propertyGroupFromAPI(v), nil
		}},
		{"blueprint", NewBlueprintResource, func(ctx context.Context, c *client.Client, key string) (any, error) {
			v, err := c.Blueprints.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			return blueprintFromAPI(v, types.StringNull()), nil
		}},
	}
}

type lookupDataSource struct {
	spec lookupSpec
	c    *client.Client
}

func (d *lookupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.spec.name
}

func (d *lookupDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var rs resource.SchemaResponse
	d.spec.resource().Schema(ctx, resource.SchemaRequest{}, &rs)
	attrs := toDataSourceAttributes(rs.Schema.Attributes)
	attrs["key"] = dschema.StringAttribute{Required: true, Description: "Key of the " + d.spec.name + " to look up."}
	resp.Schema = dschema.Schema{Description: "Looks up an existing " + d.spec.name + " by key.", Attributes: attrs}
}

func (d *lookupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *lookupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var key types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, pathKey, &key)...)
	if resp.Diagnostics.HasError() {
		return
	}
	model, err := d.spec.fetch(ctx, d.c, key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Reading %s %q failed", d.spec.name, key.ValueString()), err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// toDataSourceAttributes converts resource attributes to computed data source attributes.
func toDataSourceAttributes(in map[string]rschema.Attribute) map[string]dschema.Attribute {
	out := make(map[string]dschema.Attribute, len(in))
	for name, a := range in {
		switch a := a.(type) {
		case rschema.StringAttribute:
			out[name] = dschema.StringAttribute{Computed: true, Description: a.Description}
		case rschema.BoolAttribute:
			out[name] = dschema.BoolAttribute{Computed: true, Description: a.Description}
		case rschema.Int64Attribute:
			out[name] = dschema.Int64Attribute{Computed: true, Description: a.Description}
		case rschema.ListAttribute:
			out[name] = dschema.ListAttribute{Computed: true, ElementType: a.ElementType, Description: a.Description}
		case rschema.MapAttribute:
			out[name] = dschema.MapAttribute{Computed: true, ElementType: a.ElementType, Description: a.Description}
		case rschema.ListNestedAttribute:
			out[name] = dschema.ListNestedAttribute{Computed: true, Description: a.Description,
				NestedObject: dschema.NestedAttributeObject{Attributes: toDataSourceAttributes(a.NestedObject.Attributes)}}
		case rschema.MapNestedAttribute:
			out[name] = dschema.MapNestedAttribute{Computed: true, Description: a.Description,
				NestedObject: dschema.NestedAttributeObject{Attributes: toDataSourceAttributes(a.NestedObject.Attributes)}}
		case rschema.SingleNestedAttribute:
			out[name] = dschema.SingleNestedAttribute{Computed: true, Description: a.Description,
				Attributes: toDataSourceAttributes(a.Attributes)}
		default:
			panic(fmt.Sprintf("toDataSourceAttributes: unsupported attribute type %T for %q", a, name))
		}
	}
	return out
}

// ---- property format (read-only in the API) ----

type propertyFormatDataSource struct{ c *client.Client }

type propertyFormatModel struct {
	Key         types.String `tfsdk:"key"`
	DataType    types.String `tfsdk:"data_type"`
	ItemType    types.String `tfsdk:"item_type"`
	DisplayName types.String `tfsdk:"display_name"`
	IsDeleted   types.Bool   `tfsdk:"is_deleted"`
}

func NewPropertyFormatDataSource() datasource.DataSource { return &propertyFormatDataSource{} }

func (d *propertyFormatDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_property_format"
}

func (d *propertyFormatDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Description: "Looks up a property format. Property formats are read-only in the CMS API.",
		Attributes: map[string]dschema.Attribute{
			"key":          dschema.StringAttribute{Required: true, Description: "Key of the property format to look up, e.g. shortString."},
			"data_type":    dschema.StringAttribute{Computed: true, Description: "Underlying data type, e.g. string, array."},
			"item_type":    dschema.StringAttribute{Computed: true, Description: "Item type when data_type is array."},
			"display_name": dschema.StringAttribute{Computed: true, Description: "Name of the property format."},
			"is_deleted":   dschema.BoolAttribute{Computed: true, Description: "Whether the property format has been deleted."},
		},
	}
}

func (d *propertyFormatDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *propertyFormatDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var key types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, pathKey, &key)...)
	if resp.Diagnostics.HasError() {
		return
	}
	f, err := d.c.PropertyFormats.Get(ctx, key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Reading property format %q failed", key.ValueString()), err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, propertyFormatModel{
		Key: types.StringValue(f.Key), DataType: strOrNull(f.DataType), ItemType: strOrNull(f.ItemType),
		DisplayName: strOrNull(f.DisplayName), IsDeleted: types.BoolValue(f.IsDeleted),
	})...)
}

// ---- manifest export ----

type manifestDataSource struct{ c *client.Client }

type manifestModel struct {
	Sections        []string     `tfsdk:"sections"`
	IncludeReadOnly types.Bool   `tfsdk:"include_read_only"`
	JSON            types.String `tfsdk:"json"`
}

func NewManifestDataSource() datasource.DataSource { return &manifestDataSource{} }

func (d *manifestDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_manifest"
}

func (d *manifestDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Description: "Exports the content model (locales, property groups, content types, display templates) as a manifest. " +
			"Useful for diffing environments; importing a manifest is intentionally not supported.",
		Attributes: map[string]dschema.Attribute{
			"sections":          dschema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Sections to include; all when omitted."},
			"include_read_only": dschema.BoolAttribute{Optional: true, Description: "Include read-only (system) resources."},
			"json":              dschema.StringAttribute{Computed: true, Description: "The manifest as JSON (decode with jsondecode)."},
		},
	}
}

func (d *manifestDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *manifestDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg manifestModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, err := d.c.ExportManifest(ctx, cfg.Sections, cfg.IncludeReadOnly.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Exporting manifest failed", err.Error())
		return
	}
	cfg.JSON = types.StringValue(string(raw))
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

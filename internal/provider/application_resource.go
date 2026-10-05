package provider

import (
	"context"
	"errors"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &applicationResource{}
	_ resource.ResourceWithConfigure   = &applicationResource{}
	_ resource.ResourceWithImportState = &applicationResource{}
)

func NewApplicationResource() resource.Resource { return &applicationResource{} }

type applicationResource struct{ c *client.Client }

type applicationHostModel struct {
	Authority          types.String `tfsdk:"authority"`
	Type               types.String `tfsdk:"type"`
	Locale             types.String `tfsdk:"locale"`
	PreferredURLScheme types.String `tfsdk:"preferred_url_scheme"`
}

type applicationModel struct {
	Key                          types.String           `tfsdk:"key"`
	DisplayName                  types.String           `tfsdk:"display_name"`
	Type                         types.String           `tfsdk:"type"`
	EntryPoint                   types.String           `tfsdk:"entry_point"`
	IsDefault                    types.Bool             `tfsdk:"is_default"`
	UseApplicationSpecificAssets types.Bool             `tfsdk:"use_application_specific_assets"`
	AssetsRoot                   types.String           `tfsdk:"assets_root"`
	Hosts                        []applicationHostModel `tfsdk:"hosts"`
	UsePreviewTokens             types.Bool             `tfsdk:"use_preview_tokens"`
	PreviewURLFormats            types.Map              `tfsdk:"preview_url_formats"`
}

func (r *applicationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (r *applicationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A CMS application: a website (or remote website) with an entry point and hosts.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Required: true, Description: "Unique key. Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"display_name": schema.StringAttribute{Required: true, Description: "Name shown in the CMS."},
			"type": schema.StringAttribute{
				Required: true, Description: "website or inProcessWebsite. Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"entry_point": schema.StringAttribute{
				Required: true, Description: "Reference to the start page content, e.g. cms://content/<id>.",
			},
			"is_default": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Whether this is the default application. Defaults to false."},
			"use_application_specific_assets": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Whether the application uses a dedicated assets folder.",
			},
			"assets_root": schema.StringAttribute{
				Computed: true, Description: "Root of the application-specific assets, when enabled.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"hosts": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Hosts assigned to the application.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"authority": schema.StringAttribute{Required: true, Description: "DNS host name or IP with optional port."},
					"type": schema.StringAttribute{
						Optional: true, Computed: true,
						Description: "default, primary, preview, redirectPermanent, redirectTemporary, edit or media.",
					},
					"locale": schema.StringAttribute{Optional: true, Description: "Locale associated with this host."},
					"preferred_url_scheme": schema.StringAttribute{
						Optional: true, Computed: true, Description: "http or https.",
					},
				}},
			},
			"use_preview_tokens": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "Only applicable when type is website.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"preview_url_formats": schema.MapAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Description:   "Preview URL formats keyed by content type base or key. The server supplies a default when unset.",
				PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *applicationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func hostsToAPI(in []applicationHostModel) []client.ApplicationHost {
	var out []client.ApplicationHost
	for _, h := range in {
		out = append(out, client.ApplicationHost{
			Authority: h.Authority.ValueString(), Type: h.Type.ValueString(),
			Locale: h.Locale.ValueString(), PreferredURLScheme: h.PreferredURLScheme.ValueString(),
		})
	}
	return out
}

// stringMap converts a plan/state map to Go; unknown or null yields nil.
func stringMap(m types.Map) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := make(map[string]string, len(m.Elements()))
	for k, v := range m.Elements() {
		if s, ok := v.(types.String); ok {
			out[k] = s.ValueString()
		}
	}
	return out
}

// entryPointValue keeps the configured entry point when the API returns the same
// content item without the "?loc=..&ver=.." part, which it drops when storing the
// reference. That avoids a spurious "inconsistent result" and a permanent diff.
func entryPointValue(prior types.String, remote string) types.String {
	if !prior.IsNull() && !prior.IsUnknown() {
		pk, ok1 := client.ContentKeyFromRef(prior.ValueString())
		rk, ok2 := client.ContentKeyFromRef(remote)
		if ok1 && ok2 && pk == rk && remote == "cms://content/"+rk {
			return prior
		}
	}
	return types.StringValue(remote)
}

func applicationFromAPI(a *client.Application, priorEntryPoint types.String) applicationModel {
	m := applicationModel{
		Key:                          types.StringValue(a.Key),
		DisplayName:                  types.StringValue(a.DisplayName),
		Type:                         types.StringValue(a.Type),
		EntryPoint:                   entryPointValue(priorEntryPoint, a.EntryPoint),
		IsDefault:                    types.BoolValue(a.IsDefault),
		UseApplicationSpecificAssets: types.BoolValue(a.UseApplicationSpecificAssets),
		AssetsRoot:                   strOrNull(a.AssetsRoot),
		UsePreviewTokens:             boolVal(a.UsePreviewTokens),
		PreviewURLFormats:            types.MapNull(types.StringType),
	}
	for _, h := range a.Hosts {
		m.Hosts = append(m.Hosts, applicationHostModel{
			Authority: types.StringValue(h.Authority), Type: strOrNull(h.Type),
			Locale: strOrNull(h.Locale), PreferredURLScheme: strOrNull(h.PreferredURLScheme),
		})
	}
	if len(a.PreviewURLFormats) > 0 {
		elems := make(map[string]attr.Value, len(a.PreviewURLFormats))
		for k, v := range a.PreviewURLFormats {
			elems[k] = types.StringValue(v)
		}
		m.PreviewURLFormats = types.MapValueMust(types.StringType, elems)
	}
	return m
}

// previewFormatsPatch sends removed keys as null (merge-patch semantics).
func previewFormatsPatch(plan, state map[string]string) any {
	if len(plan) == 0 && len(state) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, v := range plan {
		out[k] = v
	}
	for k := range state {
		if _, ok := plan[k]; !ok {
			out[k] = nil
		}
	}
	return out
}

func (r *applicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.Applications.Create(ctx, plan.Key.ValueString(), &client.Application{
		Key:                          plan.Key.ValueString(),
		DisplayName:                  plan.DisplayName.ValueString(),
		Type:                         plan.Type.ValueString(),
		EntryPoint:                   plan.EntryPoint.ValueString(),
		IsDefault:                    plan.IsDefault.ValueBool(),
		UseApplicationSpecificAssets: plan.UseApplicationSpecificAssets.ValueBool(),
		Hosts:                        hostsToAPI(plan.Hosts),
		UsePreviewTokens:             boolPtr(plan.UsePreviewTokens),
		PreviewURLFormats:            stringMap(plan.PreviewURLFormats),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating application failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, applicationFromAPI(out, plan.EntryPoint))...)
}

func (r *applicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.Applications.Get(ctx, state.Key.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading application failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, applicationFromAPI(out, state.EntryPoint))...)
}

func (r *applicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state applicationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{
		"displayName":                  plan.DisplayName.ValueString(),
		"entryPoint":                   plan.EntryPoint.ValueString(),
		"isDefault":                    plan.IsDefault.ValueBool(),
		"useApplicationSpecificAssets": plan.UseApplicationSpecificAssets.ValueBool(),
		"hosts":                        nil,
		"previewUrlFormats":            previewFormatsPatch(stringMap(plan.PreviewURLFormats), stringMap(state.PreviewURLFormats)),
	}
	if h := hostsToAPI(plan.Hosts); len(h) > 0 {
		patch["hosts"] = h
	}
	if b := boolPtr(plan.UsePreviewTokens); b != nil {
		patch["usePreviewTokens"] = *b
	}
	out, err := r.c.Applications.Patch(ctx, plan.Key.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating application failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, applicationFromAPI(out, plan.EntryPoint))...)
}

func (r *applicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.Applications.Delete(ctx, state.Key.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting application failed", err.Error())
	}
}

func (r *applicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

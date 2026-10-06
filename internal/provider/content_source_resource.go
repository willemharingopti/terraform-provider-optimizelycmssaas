package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/willemharingopti/terraform-provider-optimizelycmssaas/internal/client"
)

var (
	_ resource.Resource                = &contentSourceResource{}
	_ resource.ResourceWithConfigure   = &contentSourceResource{}
	_ resource.ResourceWithImportState = &contentSourceResource{}
)

func NewContentSourceResource() resource.Resource { return &contentSourceResource{} }

type contentSourceResource struct{ c *client.Client }

type propertyMappingsModel struct {
	Key         types.String `tfsdk:"key"`
	DisplayName types.String `tfsdk:"display_name"`
	KeyFormat   types.String `tfsdk:"key_format"`
}

type contentSourceModel struct {
	Key              types.String           `tfsdk:"key"`
	Type             types.String           `tfsdk:"type"`
	SourceKey        types.String           `tfsdk:"source_key"`
	SourceType       types.String           `tfsdk:"source_type"`
	DisplayName      types.String           `tfsdk:"display_name"`
	BaseType         types.String           `tfsdk:"base_type"`
	PropertyMappings *propertyMappingsModel `tfsdk:"property_mappings"`
}

func (r *contentSourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_content_source"
}

func (r *contentSourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An external content source (e.g. a Graph source) surfaced in the CMS.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Required: true, Description: "Unique key. Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type":         schema.StringAttribute{Required: true, Description: "Source kind, e.g. graph."},
			"source_key":   schema.StringAttribute{Required: true, Description: "Key of the source this content source relates to."},
			"source_type":  schema.StringAttribute{Required: true, Description: "Source type within the source."},
			"display_name": schema.StringAttribute{Required: true, Description: "Name shown in the CMS for this source."},
			"base_type":    schema.StringAttribute{Required: true, Description: "Base type of the corresponding content type, e.g. _page."},
			"property_mappings": schema.SingleNestedAttribute{
				Required:    true,
				Description: "How source items map to CMS items.",
				Attributes: map[string]schema.Attribute{
					"key": schema.StringAttribute{
						Optional: true, Description: "Item key in the source. Required unless source_type is _Item. Cannot be changed.",
						PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
					},
					"display_name": schema.StringAttribute{Optional: true, Description: "Item display name in the CMS."},
					"key_format": schema.StringAttribute{
						Optional: true, Computed: true,
						Description:   "Format of the source identifier, e.g. digits, digitsWithHyphens.",
						PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
				},
			},
		},
	}
}

func (r *contentSourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func contentSourceFromAPI(s *client.ContentSource) contentSourceModel {
	return contentSourceModel{
		Key:         types.StringValue(s.Key),
		Type:        types.StringValue(s.Type),
		SourceKey:   types.StringValue(s.SourceKey),
		SourceType:  types.StringValue(s.SourceType),
		DisplayName: types.StringValue(s.DisplayName),
		BaseType:    types.StringValue(s.BaseType),
		PropertyMappings: &propertyMappingsModel{
			Key:         strOrNull(s.PropertyMappings.Key),
			DisplayName: strOrNull(s.PropertyMappings.DisplayName),
			KeyFormat:   strOrNull(s.PropertyMappings.KeyFormat),
		},
	}
}

func (r *contentSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan contentSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pm := plan.PropertyMappings
	out, err := r.c.ContentSources.Create(ctx, plan.Key.ValueString(), &client.ContentSource{
		Key:         plan.Key.ValueString(),
		Type:        plan.Type.ValueString(),
		SourceKey:   plan.SourceKey.ValueString(),
		SourceType:  plan.SourceType.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
		BaseType:    plan.BaseType.ValueString(),
		PropertyMappings: client.PropertyMappings{
			Key: pm.Key.ValueString(), DisplayName: pm.DisplayName.ValueString(), KeyFormat: pm.KeyFormat.ValueString(),
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating content source failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contentSourceFromAPI(out))...)
}

func (r *contentSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state contentSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.ContentSources.Get(ctx, state.Key.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading content source failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contentSourceFromAPI(out))...)
}

func (r *contentSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan contentSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pm := map[string]any{"displayName": patchStr(plan.PropertyMappings.DisplayName)}
	if !plan.PropertyMappings.KeyFormat.IsUnknown() {
		pm["keyFormat"] = patchStr(plan.PropertyMappings.KeyFormat)
	}
	out, err := r.c.ContentSources.Patch(ctx, plan.Key.ValueString(), map[string]any{
		"type":             plan.Type.ValueString(),
		"sourceKey":        plan.SourceKey.ValueString(),
		"sourceType":       plan.SourceType.ValueString(),
		"displayName":      plan.DisplayName.ValueString(),
		"baseType":         plan.BaseType.ValueString(),
		"propertyMappings": pm,
	})
	if err != nil {
		resp.Diagnostics.AddError("Updating content source failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contentSourceFromAPI(out))...)
}

func (r *contentSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state contentSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.ContentSources.Delete(ctx, state.Key.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting content source failed", err.Error())
	}
}

func (r *contentSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

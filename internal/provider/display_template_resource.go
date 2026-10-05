package provider

import (
	"context"
	"errors"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &displayTemplateResource{}
	_ resource.ResourceWithConfigure   = &displayTemplateResource{}
	_ resource.ResourceWithImportState = &displayTemplateResource{}
)

func NewDisplayTemplateResource() resource.Resource { return &displayTemplateResource{} }

type displayTemplateResource struct{ c *client.Client }

type displayChoiceModel struct {
	DisplayName types.String `tfsdk:"display_name"`
	SortOrder   types.Int64  `tfsdk:"sort_order"`
}

type displaySettingModel struct {
	DisplayName types.String                  `tfsdk:"display_name"`
	Editor      types.String                  `tfsdk:"editor"`
	SortOrder   types.Int64                   `tfsdk:"sort_order"`
	Choices     map[string]displayChoiceModel `tfsdk:"choices"`
}

type displayTemplateModel struct {
	Key         types.String                   `tfsdk:"key"`
	DisplayName types.String                   `tfsdk:"display_name"`
	NodeType    types.String                   `tfsdk:"node_type"`
	BaseType    types.String                   `tfsdk:"base_type"`
	ContentType types.String                   `tfsdk:"content_type"`
	IsDefault   types.Bool                     `tfsdk:"is_default"`
	Settings    map[string]displaySettingModel `tfsdk:"settings"`
}

func (r *displayTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_display_template"
}

func (r *displayTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	sortOrder := schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), Description: "Position among siblings; lower comes first. Defaults to 0."}
	resp.Schema = schema.Schema{
		Description: "A display template: editor-selectable settings (and choices) for a base type, node type or content type.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Required: true, Description: "Unique key. Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"display_name": schema.StringAttribute{Required: true, Description: "Name shown to editors."},
			"node_type":    schema.StringAttribute{Optional: true, Description: "Node type this template is valid for."},
			"base_type":    schema.StringAttribute{Optional: true, Description: "Base type this template is valid for, e.g. _component."},
			"content_type": schema.StringAttribute{Optional: true, Description: "Key of the content type this template is valid for."},
			"is_default": schema.BoolAttribute{
				Optional: true, Computed: true,
				Description: "Default template for the associated base type, node type or content type. " +
					"The CMS makes the first template for a type the default regardless of this value; leave it unset to accept that.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"settings": schema.MapNestedAttribute{
				Optional:    true,
				Description: "Settings keyed by setting key.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"display_name": schema.StringAttribute{Required: true, Description: "Name of the setting shown to editors."},
					"editor":       schema.StringAttribute{Optional: true, Description: "Suggested editor, e.g. select."},
					"sort_order":   sortOrder,
					"choices": schema.MapNestedAttribute{
						Optional:    true,
						Description: "Selectable choices keyed by choice key.",
						NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
							"display_name": schema.StringAttribute{Required: true, Description: "Name of the choice shown to editors."},
							"sort_order":   sortOrder,
						}},
					},
				}},
			},
		},
	}
}

func (r *displayTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func settingsToAPI(in map[string]displaySettingModel) map[string]client.DisplaySetting {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]client.DisplaySetting, len(in))
	for k, s := range in {
		ds := client.DisplaySetting{
			DisplayName: s.DisplayName.ValueString(), Editor: s.Editor.ValueString(), SortOrder: int32(s.SortOrder.ValueInt64()),
		}
		if len(s.Choices) > 0 {
			ds.Choices = make(map[string]client.DisplaySettingChoice, len(s.Choices))
			for ck, c := range s.Choices {
				ds.Choices[ck] = client.DisplaySettingChoice{DisplayName: c.DisplayName.ValueString(), SortOrder: int32(c.SortOrder.ValueInt64())}
			}
		}
		out[k] = ds
	}
	return out
}

// settingsPatch builds a merge-patch body: entries that disappeared from the
// plan (settings, or choices within a kept setting) are sent as null.
func settingsPatch(plan, state map[string]displaySettingModel) any {
	if len(plan) == 0 && len(state) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, s := range settingsToAPI(plan) {
		m := map[string]any{"displayName": s.DisplayName, "editor": nil, "sortOrder": s.SortOrder}
		if s.Editor != "" {
			m["editor"] = s.Editor
		}
		choices := map[string]any{}
		for ck, c := range s.Choices {
			choices[ck] = c
		}
		for ck := range state[k].Choices {
			if _, ok := s.Choices[ck]; !ok {
				choices[ck] = nil
			}
		}
		if len(choices) > 0 {
			m["choices"] = choices
		}
		out[k] = m
	}
	for k := range state {
		if _, ok := plan[k]; !ok {
			out[k] = nil
		}
	}
	return out
}

func displayTemplateFromAPI(t *client.DisplayTemplate) displayTemplateModel {
	m := displayTemplateModel{
		Key:         types.StringValue(t.Key),
		DisplayName: types.StringValue(t.DisplayName),
		NodeType:    strOrNull(t.NodeType),
		BaseType:    strOrNull(t.BaseType),
		ContentType: strOrNull(t.ContentType),
		IsDefault:   boolVal(t.IsDefault),
	}
	if len(t.Settings) > 0 {
		m.Settings = make(map[string]displaySettingModel, len(t.Settings))
		for k, s := range t.Settings {
			sm := displaySettingModel{
				DisplayName: types.StringValue(s.DisplayName), Editor: strOrNull(s.Editor), SortOrder: types.Int64Value(int64(s.SortOrder)),
			}
			if len(s.Choices) > 0 {
				sm.Choices = make(map[string]displayChoiceModel, len(s.Choices))
				for ck, c := range s.Choices {
					sm.Choices[ck] = displayChoiceModel{DisplayName: types.StringValue(c.DisplayName), SortOrder: types.Int64Value(int64(c.SortOrder))}
				}
			}
			m.Settings[k] = sm
		}
	}
	return m
}

func (r *displayTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan displayTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.DisplayTemplates.Create(ctx, plan.Key.ValueString(), &client.DisplayTemplate{
		Key:         plan.Key.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
		NodeType:    plan.NodeType.ValueString(),
		BaseType:    plan.BaseType.ValueString(),
		ContentType: plan.ContentType.ValueString(),
		IsDefault:   boolPtr(plan.IsDefault),
		Settings:    settingsToAPI(plan.Settings),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating display template failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, displayTemplateFromAPI(out))...)
}

func (r *displayTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state displayTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.DisplayTemplates.Get(ctx, state.Key.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading display template failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, displayTemplateFromAPI(out))...)
}

func (r *displayTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state displayTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{
		"displayName": plan.DisplayName.ValueString(),
		"nodeType":    patchStr(plan.NodeType),
		"baseType":    patchStr(plan.BaseType),
		"contentType": patchStr(plan.ContentType),
		"settings":    settingsPatch(plan.Settings, state.Settings),
	}
	if b := boolPtr(plan.IsDefault); b != nil {
		patch["isDefault"] = *b
	}
	out, err := r.c.DisplayTemplates.Patch(ctx, plan.Key.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating display template failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, displayTemplateFromAPI(out))...)
}

func (r *displayTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state displayTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.DisplayTemplates.Delete(ctx, state.Key.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting display template failed", err.Error())
	}
}

func (r *displayTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

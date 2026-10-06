package provider

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/willemharingopti/terraform-provider-optimizelycmssaas/internal/client"
)

var (
	_ resource.Resource                = &contentTypeResource{}
	_ resource.ResourceWithConfigure   = &contentTypeResource{}
	_ resource.ResourceWithImportState = &contentTypeResource{}
)

func NewContentTypeResource() resource.Resource { return &contentTypeResource{} }

type contentTypeResource struct{ c *client.Client }

type contentTypeModel struct {
	Key                  types.String       `tfsdk:"key"`
	DisplayName          types.String       `tfsdk:"display_name"`
	Description          types.String       `tfsdk:"description"`
	BaseType             types.String       `tfsdk:"base_type"`
	IsContract           types.Bool         `tfsdk:"is_contract"`
	SortOrder            types.Int64        `tfsdk:"sort_order"`
	MayContainTypes      []string           `tfsdk:"may_contain_types"`
	MediaFileExtensions  []string           `tfsdk:"media_file_extensions"`
	CompositionBehaviors []string           `tfsdk:"composition_behaviors"`
	Contracts            []string           `tfsdk:"contracts"`
	PropertiesJSON       types.String       `tfsdk:"properties_json"`
	AllowDataLoss        types.Bool         `tfsdk:"allow_data_loss"`
	AccessRights         []accessRightModel `tfsdk:"access_rights"`
}

func (r *contentTypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_content_type"
}

func (r *contentTypeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	strList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: desc}
	}
	resp.Schema = schema.Schema{
		Description: "An Optimizely CMS (SaaS) content type.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Required: true, Description: "Unique key (letters, digits, underscore; starts with a letter). Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"display_name": schema.StringAttribute{Required: true, Description: "Name shown in the CMS."},
			"description":  schema.StringAttribute{Optional: true, Description: "Description of the content type (at most 255 characters)."},
			"base_type": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Base type, e.g. _page, _component, _media, _experience. Required unless is_contract. Cannot be changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace(),
				},
			},
			"is_contract": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description:   "Whether this is a contract type. Cannot be changed.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"sort_order": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Position when sorting content types. Assigned by the CMS when unset.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"may_contain_types":     strList("Content types that can be created in containers of this type."),
			"media_file_extensions": strList("Media file extensions this type handles."),
			"composition_behaviors": strList("e.g. sectionEnabled, elementEnabled."),
			"contracts":             strList("Contract types this type is bound to."),
			"properties_json": schema.StringAttribute{
				Optional: true,
				Description: "JSON object of property definitions keyed by property name (use jsonencode). " +
					"Drift is detected structurally; fields the API adds as defaults are ignored.",
			},
			"allow_data_loss": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Let updates go through even when the CMS considers them breaking, e.g. removing a property " +
					"that content may use. That deletes the stored values. Off by default: the apply then fails with the CMS's message.",
			},
			"access_rights": accessRightsAttribute("Who may create content of this type. Empty means everyone."),
		},
	}
}

func (r *contentTypeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func parseProps(s types.String) (map[string]json.RawMessage, error) {
	if s.IsNull() || s.IsUnknown() || s.ValueString() == "" {
		return nil, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s.ValueString()), &m); err != nil {
		return nil, errors.New("properties_json must be a JSON object: " + err.Error())
	}
	return m, nil
}

// contentTypeFromAPI builds state from the API response, keeping the prior
// properties_json when it is structurally equivalent so formatting never drifts.
func contentTypeFromAPI(ct *client.ContentType, prior contentTypeModel) contentTypeModel {
	m := contentTypeModel{
		Key:                  types.StringValue(ct.Key),
		DisplayName:          types.StringValue(ct.DisplayName),
		Description:          strOrNull(ct.Description),
		BaseType:             prior.BaseType,
		IsContract:           types.BoolValue(ct.IsContract),
		SortOrder:            int32Val(ct.SortOrder),
		MayContainTypes:      listOrNil(ct.MayContainTypes),
		MediaFileExtensions:  listOrNil(ct.MediaFileExtensions),
		CompositionBehaviors: listOrNil(ct.CompositionBehaviors),
		Contracts:            listOrNil(ct.Contracts),
		AccessRights:         accessRightsFromAPI(ct.AccessRights),
		PropertiesJSON:       prior.PropertiesJSON,
		AllowDataLoss:        prior.AllowDataLoss,
	}
	if ct.BaseType != "" {
		m.BaseType = types.StringValue(ct.BaseType)
	} else if m.BaseType.IsUnknown() {
		m.BaseType = types.StringNull()
	}
	switch {
	case len(ct.Properties) == 0 && (prior.PropertiesJSON.IsNull() || prior.PropertiesJSON.ValueString() == ""):
		m.PropertiesJSON = types.StringNull()
	case prior.PropertiesJSON.IsNull() || prior.PropertiesJSON.IsUnknown() ||
		!propertiesEquivalent(prior.PropertiesJSON.ValueString(), ct.Properties):
		b, _ := json.Marshal(ct.Properties)
		m.PropertiesJSON = types.StringValue(string(b))
	}
	return m
}

func (r *contentTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan contentTypeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	props, err := parseProps(plan.PropertiesJSON)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("properties_json"), "Invalid JSON", err.Error())
		return
	}
	ct, err := r.c.ContentTypes.Create(ctx, plan.Key.ValueString(), &client.ContentType{
		Key:                  plan.Key.ValueString(),
		DisplayName:          plan.DisplayName.ValueString(),
		Description:          plan.Description.ValueString(),
		BaseType:             plan.BaseType.ValueString(),
		IsContract:           plan.IsContract.ValueBool(),
		SortOrder:            int64Ptr(plan.SortOrder),
		MayContainTypes:      plan.MayContainTypes,
		MediaFileExtensions:  plan.MediaFileExtensions,
		CompositionBehaviors: plan.CompositionBehaviors,
		Contracts:            plan.Contracts,
		Properties:           props,
		AccessRights:         accessRightsToAPI(plan.AccessRights),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating content type failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contentTypeFromAPI(ct, plan))...)
}

func (r *contentTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state contentTypeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ct, err := r.c.ContentTypes.Get(ctx, state.Key.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading content type failed", err.Error())
		return
	}
	if state.AllowDataLoss.IsNull() {
		state.AllowDataLoss = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contentTypeFromAPI(ct, state))...)
}

func (r *contentTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state contentTypeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	newProps, err1 := parseProps(plan.PropertiesJSON)
	oldProps, err2 := parseProps(state.PropertiesJSON)
	if err := errors.Join(err1, err2); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("properties_json"), "Invalid JSON", err.Error())
		return
	}
	// Merge-patch: an explicit null removes a property that left the config.
	props := map[string]any{}
	for k, v := range newProps {
		props[k] = v
	}
	for k := range oldProps {
		if _, ok := newProps[k]; !ok {
			props[k] = nil
		}
	}
	patch := map[string]any{
		"displayName":          plan.DisplayName.ValueString(),
		"description":          patchStr(plan.Description),
		"mayContainTypes":      patchList(plan.MayContainTypes),
		"mediaFileExtensions":  patchList(plan.MediaFileExtensions),
		"compositionBehaviors": patchList(plan.CompositionBehaviors),
		"contracts":            patchList(plan.Contracts),
		"accessRights":         patchAccessRights(plan.AccessRights),
	}
	if len(props) > 0 {
		patch["properties"] = props
	}
	if so := int64Ptr(plan.SortOrder); so != nil {
		patch["sortOrder"] = *so
	}
	var headers map[string]string
	if plan.AllowDataLoss.ValueBool() {
		headers = map[string]string{"cms-ignore-data-loss-warnings": "true"}
	}
	ct, err := r.c.ContentTypes.PatchH(ctx, plan.Key.ValueString(), patch, headers)
	if err != nil {
		resp.Diagnostics.AddError("Updating content type failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, contentTypeFromAPI(ct, plan))...)
}

func (r *contentTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state contentTypeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.ContentTypes.Delete(ctx, state.Key.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting content type failed", err.Error())
	}
}

func (r *contentTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

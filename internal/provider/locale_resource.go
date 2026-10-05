package provider

import (
	"context"
	"errors"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &localeResource{}
	_ resource.ResourceWithConfigure   = &localeResource{}
	_ resource.ResourceWithImportState = &localeResource{}
)

func NewLocaleResource() resource.Resource { return &localeResource{} }

type localeResource struct{ c *client.Client }

type localeModel struct {
	Key          types.String       `tfsdk:"key"`
	DisplayName  types.String       `tfsdk:"display_name"`
	RouteSegment types.String       `tfsdk:"route_segment"`
	IsEnabled    types.Bool         `tfsdk:"is_enabled"`
	SortOrder    types.Int64        `tfsdk:"sort_order"`
	Fallback     types.String       `tfsdk:"fallback"`
	AccessRights []accessRightModel `tfsdk:"access_rights"`
}

func (r *localeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_locale"
}

func (r *localeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A language/locale that content can be created in.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Required: true, Description: "IETF BCP-47 language tag, e.g. en, en-US, sv-SE. Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"display_name":  schema.StringAttribute{Required: true, Description: "Name shown in the CMS."},
			"route_segment": schema.StringAttribute{Required: true, Description: "URL segment used when routing to this locale."},
			"is_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "Whether content can be created in this locale. The CMS chooses a value when unset.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"sort_order": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Position when sorting locales. Assigned by the CMS when unset.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"fallback":      schema.StringAttribute{Optional: true, Description: "Locale key to fall back to when content is missing."},
			"access_rights": accessRightsAttribute("Who may create content in this locale. Empty means everyone."),
		},
	}
}

func (r *localeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func localeFromAPI(l *client.Locale) localeModel {
	m := localeModel{
		Key:          types.StringValue(l.Key),
		DisplayName:  types.StringValue(l.DisplayName),
		RouteSegment: types.StringValue(l.RouteSegment),
		IsEnabled:    boolVal(l.IsEnabled),
		SortOrder:    int32Val(l.SortOrder),
		Fallback:     types.StringNull(),
		AccessRights: accessRightsFromAPI(l.AccessRights),
	}
	if l.Fallback != nil {
		m.Fallback = strOrNull(*l.Fallback)
	}
	return m
}

func (r *localeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan localeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := &client.Locale{
		Key:          plan.Key.ValueString(),
		DisplayName:  plan.DisplayName.ValueString(),
		RouteSegment: plan.RouteSegment.ValueString(),
		IsEnabled:    boolPtr(plan.IsEnabled),
		SortOrder:    int64Ptr(plan.SortOrder),
		AccessRights: accessRightsToAPI(plan.AccessRights),
	}
	if !plan.Fallback.IsNull() {
		f := plan.Fallback.ValueString()
		in.Fallback = &f
	}
	out, err := r.c.Locales.Create(ctx, in.Key, in)
	if err != nil {
		resp.Diagnostics.AddError("Creating locale failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, localeFromAPI(out))...)
}

func (r *localeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state localeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.Locales.Get(ctx, state.Key.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading locale failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, localeFromAPI(out))...)
}

func (r *localeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan localeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{
		"displayName":  plan.DisplayName.ValueString(),
		"routeSegment": plan.RouteSegment.ValueString(),
		"fallback":     patchStr(plan.Fallback),
		"accessRights": patchAccessRights(plan.AccessRights),
	}
	if b := boolPtr(plan.IsEnabled); b != nil {
		patch["isEnabled"] = *b
	}
	if so := int64Ptr(plan.SortOrder); so != nil {
		patch["sortOrder"] = *so
	}
	out, err := r.c.Locales.Patch(ctx, plan.Key.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating locale failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, localeFromAPI(out))...)
}

func (r *localeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state localeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.Locales.Delete(ctx, state.Key.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting locale failed", err.Error())
	}
}

func (r *localeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

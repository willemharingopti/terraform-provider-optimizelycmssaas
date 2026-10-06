package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/willemharingopti/terraform-provider-optimizelycmssaas/internal/client"
)

var (
	_ resource.Resource                = &propertyGroupResource{}
	_ resource.ResourceWithConfigure   = &propertyGroupResource{}
	_ resource.ResourceWithImportState = &propertyGroupResource{}
)

func NewPropertyGroupResource() resource.Resource { return &propertyGroupResource{} }

type propertyGroupResource struct{ c *client.Client }

type propertyGroupModel struct {
	Key         types.String `tfsdk:"key"`
	DisplayName types.String `tfsdk:"display_name"`
	SortOrder   types.Int64  `tfsdk:"sort_order"`
}

func (r *propertyGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_property_group"
}

func (r *propertyGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A property group used to organise content type properties in the editor.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Required: true, Description: "Unique key. Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"display_name": schema.StringAttribute{Required: true, Description: "Name shown in the editor."},
			"sort_order": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Position when sorting property groups. Assigned by the CMS when unset.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *propertyGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func propertyGroupFromAPI(g *client.PropertyGroup) propertyGroupModel {
	return propertyGroupModel{Key: types.StringValue(g.Key), DisplayName: types.StringValue(g.DisplayName), SortOrder: int32Val(g.SortOrder)}
}

func (r *propertyGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan propertyGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.PropertyGroups.Create(ctx, plan.Key.ValueString(), &client.PropertyGroup{
		Key: plan.Key.ValueString(), DisplayName: plan.DisplayName.ValueString(), SortOrder: int64Ptr(plan.SortOrder),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating property group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, propertyGroupFromAPI(out))...)
}

func (r *propertyGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state propertyGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.PropertyGroups.Get(ctx, state.Key.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading property group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, propertyGroupFromAPI(out))...)
}

func (r *propertyGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan propertyGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{"displayName": plan.DisplayName.ValueString()}
	if so := int64Ptr(plan.SortOrder); so != nil {
		patch["sortOrder"] = *so
	}
	out, err := r.c.PropertyGroups.Patch(ctx, plan.Key.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Updating property group failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, propertyGroupFromAPI(out))...)
}

func (r *propertyGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state propertyGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.PropertyGroups.Delete(ctx, state.Key.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting property group failed", err.Error())
	}
}

func (r *propertyGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

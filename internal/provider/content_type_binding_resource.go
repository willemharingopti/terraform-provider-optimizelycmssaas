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
	_ resource.Resource                = &contentTypeBindingResource{}
	_ resource.ResourceWithConfigure   = &contentTypeBindingResource{}
	_ resource.ResourceWithImportState = &contentTypeBindingResource{}
)

func NewContentTypeBindingResource() resource.Resource { return &contentTypeBindingResource{} }

type contentTypeBindingResource struct{ c *client.Client }

type propertyMappingModel struct {
	From    types.String `tfsdk:"from"`
	Binding types.String `tfsdk:"binding"`
}

type contentTypeBindingModel struct {
	Key              types.String                    `tfsdk:"key"`
	From             types.String                    `tfsdk:"from"`
	To               types.String                    `tfsdk:"to"`
	PropertyMappings map[string]propertyMappingModel `tfsdk:"property_mappings"`
}

func (r *contentTypeBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_content_type_binding"
}

func (r *contentTypeBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A binding that maps one content type (and its properties) onto another, e.g. for external content sources.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Required: true, Description: "Unique key. Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"from": schema.StringAttribute{Required: true, Description: "Key of the content type bound from."},
			"to":   schema.StringAttribute{Required: true, Description: "Key of the content type bound to."},
			"property_mappings": schema.MapNestedAttribute{
				Optional:    true,
				Description: "Property mappings. The map key is the path the property binds to.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"from":    schema.StringAttribute{Required: true, Description: "Path of the property on the source content type."},
					"binding": schema.StringAttribute{Optional: true, Description: "Key of an existing binding to reuse for this property."},
				}},
			},
		},
	}
}

func (r *contentTypeBindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func bindingMappingsToAPI(in map[string]propertyMappingModel) map[string]client.PropertyMapping {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]client.PropertyMapping, len(in))
	for k, v := range in {
		out[k] = client.PropertyMapping{From: v.From.ValueString(), Binding: v.Binding.ValueString()}
	}
	return out
}

func bindingFromAPI(b *client.ContentTypeBinding) contentTypeBindingModel {
	m := contentTypeBindingModel{Key: types.StringValue(b.Key), From: types.StringValue(b.From), To: types.StringValue(b.To)}
	if len(b.PropertyMappings) > 0 {
		m.PropertyMappings = make(map[string]propertyMappingModel, len(b.PropertyMappings))
		for k, v := range b.PropertyMappings {
			m.PropertyMappings[k] = propertyMappingModel{From: types.StringValue(v.From), Binding: strOrNull(v.Binding)}
		}
	}
	return m
}

func (r *contentTypeBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan contentTypeBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.ContentTypeBindings.Create(ctx, plan.Key.ValueString(), &client.ContentTypeBinding{
		Key: plan.Key.ValueString(), From: plan.From.ValueString(), To: plan.To.ValueString(),
		PropertyMappings: bindingMappingsToAPI(plan.PropertyMappings),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating content type binding failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, bindingFromAPI(out))...)
}

func (r *contentTypeBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state contentTypeBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.ContentTypeBindings.Get(ctx, state.Key.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading content type binding failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, bindingFromAPI(out))...)
}

func (r *contentTypeBindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state contentTypeBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Merge-patch: mappings dropped from config are sent as null to delete them.
	var mappings any
	if len(plan.PropertyMappings) > 0 || len(state.PropertyMappings) > 0 {
		m := map[string]any{}
		for k, v := range bindingMappingsToAPI(plan.PropertyMappings) {
			m[k] = v
		}
		for k := range state.PropertyMappings {
			if _, ok := plan.PropertyMappings[k]; !ok {
				m[k] = nil
			}
		}
		mappings = m
	}
	out, err := r.c.ContentTypeBindings.Patch(ctx, plan.Key.ValueString(), map[string]any{
		"from": plan.From.ValueString(), "to": plan.To.ValueString(), "propertyMappings": mappings,
	})
	if err != nil {
		resp.Diagnostics.AddError("Updating content type binding failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, bindingFromAPI(out))...)
}

func (r *contentTypeBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state contentTypeBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.ContentTypeBindings.Delete(ctx, state.Key.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting content type binding failed", err.Error())
	}
}

func (r *contentTypeBindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

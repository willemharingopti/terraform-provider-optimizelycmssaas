package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A model struct that disagrees with its schema only fails at runtime in
// Terraform; these tests surface it in CI instead.

func setOn(t *testing.T, name string, ctor func() resource.Resource, model any) {
	t.Helper()
	var rs resource.SchemaResponse
	ctor().Schema(context.Background(), resource.SchemaRequest{}, &rs)
	st := tfsdk.State{Schema: rs.Schema, Raw: tftypes.NewValue(rs.Schema.Type().TerraformType(context.Background()), nil)}
	if diags := st.Set(context.Background(), model); diags.HasError() {
		t.Errorf("%s: model does not match schema: %v", name, diags)
	}
}

func TestResourceModelsMatchSchemas(t *testing.T) {
	cases := []struct {
		name  string
		ctor  func() resource.Resource
		model any
	}{
		{"content_type", NewContentTypeResource, contentTypeModel{}},
		{"content_source", NewContentSourceResource, contentSourceModel{PropertyMappings: &propertyMappingsModel{}}},
		{"content_type_binding", NewContentTypeBindingResource, contentTypeBindingModel{}},
		{"locale", NewLocaleResource, localeModel{}},
		{"display_template", NewDisplayTemplateResource, displayTemplateModel{}},
		{"application", NewApplicationResource, applicationModel{PreviewURLFormats: types.MapNull(types.StringType)}},
		{"property_group", NewPropertyGroupResource, propertyGroupModel{}},
		{"blueprint", NewBlueprintResource, blueprintModel{}},
		{"content", NewContentResource, contentModel{}},
	}
	for _, tc := range cases {
		setOn(t, tc.name, tc.ctor, tc.model)
	}
}

func TestLookupDataSourcesMatchModels(t *testing.T) {
	for _, spec := range lookupSpecs() {
		ds := &lookupDataSource{spec: spec}
		var resp datasource.SchemaResponse
		ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: %v", spec.name, resp.Diagnostics)
		}
		if _, ok := resp.Schema.Attributes["key"]; !ok || !resp.Schema.Attributes["key"].IsRequired() {
			t.Errorf("%s: key must be required", spec.name)
		}
	}
}

// Realistic round trips using shapes captured from a real instance.
func TestApplicationFromRealShape(t *testing.T) {
	const real = `{"key":"novobanco","displayName":"novobanco","type":"website","entryPoint":"cms://content/74ff796bf9cd4e0eb53e08544b755376",
	"isDefault":false,"useApplicationSpecificAssets":true,"assetsRoot":"cms://content/0a1cc60f2a674bd38da1c94528d33bd4",
	"hosts":[{"authority":"localhost:7777","type":"primary","preferredUrlScheme":"https"}],"usePreviewTokens":true,
	"previewUrlFormats":{"any":"{host}/preview?key={key}"}}`
	var a client.Application
	if err := json.Unmarshal([]byte(real), &a); err != nil {
		t.Fatal(err)
	}
	m := applicationFromAPI(&a, types.StringNull())
	if m.Hosts[0].Locale.IsNull() != true || m.Hosts[0].Type.ValueString() != "primary" || m.AssetsRoot.IsNull() {
		t.Errorf("unexpected mapping: %+v", m)
	}
	setOn(t, "application(real)", NewApplicationResource, m)
}

func TestDisplayTemplateFromRealShape(t *testing.T) {
	const real = `{"key":"DefaultButton","displayName":"Button Default","contentType":"Button","isDefault":true,
	"settings":{"transform":{"displayName":"Text Transform","editor":"select","sortOrder":5,
	"choices":{"keep":{"displayName":"As entered","sortOrder":10},"uppercase":{"displayName":"Uppercase","sortOrder":20}}}}}`
	var d client.DisplayTemplate
	if err := json.Unmarshal([]byte(real), &d); err != nil {
		t.Fatal(err)
	}
	m := displayTemplateFromAPI(&d)
	if len(m.Settings["transform"].Choices) != 2 || m.BaseType.IsNull() != true {
		t.Errorf("unexpected mapping: %+v", m)
	}
	setOn(t, "display_template(real)", NewDisplayTemplateResource, m)
}

func TestContentFromAPIKeepsEquivalentPropertiesJSON(t *testing.T) {
	node := &client.ContentNode{Key: "k1", ContentType: "Article", Container: "root"}
	ver := &client.ContentVersion{
		Version: "v1", Status: "draft", Locale: "en", DisplayName: "Hello",
		Properties: map[string]json.RawMessage{"title": json.RawMessage(`"Hi"`), "extra": json.RawMessage(`{"a":1}`)},
	}
	prior := contentModel{PropertiesJSON: types.StringValue(`{"title":"Hi","extra":{"a":1}}`), Publish: types.BoolValue(true)}
	got := contentFromAPI(node, ver, prior)
	if got.PropertiesJSON.ValueString() != prior.PropertiesJSON.ValueString() {
		t.Errorf("equivalent JSON should be kept verbatim, got %s", got.PropertiesJSON.ValueString())
	}
	ver.Properties["title"] = json.RawMessage(`"Changed"`)
	got = contentFromAPI(node, ver, prior)
	if got.PropertiesJSON.ValueString() == prior.PropertiesJSON.ValueString() {
		t.Error("changed remote value must surface as drift")
	}
	setOn(t, "content(real)", NewContentResource, got)
}

func TestMergePatchDiff(t *testing.T) {
	oldV := map[string]any{"a": 1.0, "b": map[string]any{"x": 1.0, "y": 2.0}, "gone": "z"}
	newV := map[string]any{"a": 1.0, "b": map[string]any{"x": 1.0, "y": 3.0}, "new": true}
	b, _ := json.Marshal(mergePatchDiff(oldV, newV))
	if string(b) != `{"b":{"y":3},"gone":null,"new":true}` {
		t.Errorf("got %s", b)
	}
	if len(mergePatchDiff(oldV, oldV)) != 0 {
		t.Error("identical input should produce an empty patch")
	}
}

func TestReconcileJSON(t *testing.T) {
	remote := map[string]any{"properties": map[string]any{"a": "1"}, "composition": nil}
	prior := types.StringValue(`{"properties":{"a":"1"}}`)
	if got := reconcileJSON(prior, remote); got.ValueString() != prior.ValueString() {
		t.Errorf("subset should keep prior, got %s", got.ValueString())
	}
	if got := reconcileJSON(types.StringNull(), map[string]any{}); !got.IsNull() {
		t.Error("null prior + empty remote should stay null")
	}
	if got := reconcileJSON(types.StringNull(), remote); got.IsNull() {
		t.Error("null prior + non-empty remote should import remote")
	}
}

func TestEntryPointValueKeepsConfiguredFormWhenAPIDropsTheLocale(t *testing.T) {
	configured := types.StringValue("cms://content/abc123?loc=en")
	if got := entryPointValue(configured, "cms://content/abc123"); got != configured {
		t.Errorf("same item without ?loc must keep the configured value, got %s", got)
	}
	if got := entryPointValue(configured, "cms://content/other"); got.ValueString() != "cms://content/other" {
		t.Errorf("a different item must surface as drift, got %s", got)
	}
	if got := entryPointValue(configured, "cms://content/abc123?loc=sv"); got.ValueString() != "cms://content/abc123?loc=sv" {
		t.Errorf("if the API does return a query it must be shown as is, got %s", got)
	}
	if got := entryPointValue(types.StringNull(), "cms://content/abc123"); got.ValueString() != "cms://content/abc123" {
		t.Errorf("import must adopt the remote value, got %s", got)
	}
}

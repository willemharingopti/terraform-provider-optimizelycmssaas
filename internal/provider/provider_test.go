package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// TestSchemasAreValid catches schema mistakes (bad defaults, invalid attribute
// combinations) that the framework otherwise only reports at runtime.
func TestSchemasAreValid(t *testing.T) {
	srv := providerserver.NewProtocol6(New("test")())()
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		t.Errorf("%s: %s", d.Summary, d.Detail)
	}
	for _, name := range []string{"optimizelycmssaas_content_type", "optimizelycmssaas_content_source", "optimizelycmssaas_content_type_binding", "optimizelycmssaas_locale", "optimizelycmssaas_display_template", "optimizelycmssaas_application", "optimizelycmssaas_property_group", "optimizelycmssaas_blueprint", "optimizelycmssaas_content"} {
		if resp.ResourceSchemas[name] == nil {
			t.Errorf("resource %s not registered", name)
		}
	}
	for _, name := range []string{"optimizelycmssaas_locale", "optimizelycmssaas_content_type", "optimizelycmssaas_application", "optimizelycmssaas_property_format", "optimizelycmssaas_manifest", "optimizelycmssaas_content_root",
		"optimizelycmssaas_content_source", "optimizelycmssaas_content_type_binding", "optimizelycmssaas_display_template", "optimizelycmssaas_property_group", "optimizelycmssaas_blueprint"} {
		if resp.DataSourceSchemas[name] == nil {
			t.Errorf("data source %s not registered", name)
		}
	}
}

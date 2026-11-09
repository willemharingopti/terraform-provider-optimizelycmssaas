package provider

// Acceptance tests run against a REAL Optimizely CMS instance and create, change
// and delete objects there. They are skipped unless TF_ACC=1.
//
//	TF_ACC=1 TF_ACC_TERRAFORM_PATH=$(which terraform) \
//	OPTIMIZELY_CMS_CLIENT_ID=... OPTIMIZELY_CMS_CLIENT_SECRET=... \
//	go test ./internal/provider -run TestAcc -v -timeout 30m
//
// All objects use the "TfAcc" key prefix (locale: "gd") and are destroyed at the
// end. Use a sandbox instance. Content and application tests also need:
//
//	OPTIMIZELY_CMS_TEST_CONTAINER, OPTIMIZELY_CMS_TEST_CONTENT_TYPE  (content, application)
//	(a localized page type that may be created in that container)
//
// The composition test also needs OPTIMIZELY_CMS_TEST_ELEMENT_TYPE: a component type enabled for
// composition (elementEnabled) without required properties. OPTIMIZELY_CMS_TEST_EXPERIENCE_TYPE
// defaults to BlankExperience and must be creatable in the container.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/willemharingopti/terraform-provider-optimizelycmssaas/internal/client"
)

var testAccProviders = map[string]func() (tfprotov6.ProviderServer, error){
	"optimizelycmssaas": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, v := range []string{"OPTIMIZELY_CMS_CLIENT_ID", "OPTIMIZELY_CMS_CLIENT_SECRET"} {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for acceptance tests", v)
		}
	}
}

func testAccEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if os.Getenv(n) == "" {
			t.Skipf("%s not set", n)
		}
	}
}

func testAccClient() (*client.Client, error) {
	return client.New(os.Getenv("OPTIMIZELY_CMS_BASE_URL"), os.Getenv("OPTIMIZELY_CMS_TOKEN_URL"),
		os.Getenv("OPTIMIZELY_CMS_CLIENT_ID"), os.Getenv("OPTIMIZELY_CMS_CLIENT_SECRET"))
}

// destroyed asserts every resource of tfType is gone from the API.
func destroyed(tfType string, get func(ctx context.Context, c *client.Client, key string) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, err := testAccClient()
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != tfType {
				continue
			}
			err := get(context.Background(), c, rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("%s %q still exists", tfType, rs.Primary.ID)
			}
			if !errors.Is(err, client.ErrNotFound) {
				return err
			}
		}
		return nil
	}
}

// keyedImport is an import step addressed by the resource's key attribute.
func keyedImport(name string, ignore ...string) resource.TestStep {
	return resource.TestStep{
		ResourceName: name, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: ignore,
		ImportStateVerifyIdentifierAttribute: "key",
		ImportStateIdFunc: func(s *terraform.State) (string, error) {
			return s.RootModule().Resources[name].Primary.Attributes["key"], nil
		},
	}
}

func TestAccLocale(t *testing.T) {
	cfg := func(name string, enabled bool) string {
		return fmt.Sprintf(`
resource "optimizelycmssaas_locale" "t" {
  key           = "gd"
  display_name  = %q
  route_segment = "gd"
  is_enabled    = %t
}`, name, enabled)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: destroyed("optimizelycmssaas_locale", func(ctx context.Context, c *client.Client, k string) error {
			_, err := c.Locales.Get(ctx, k)
			return err
		}),
		Steps: []resource.TestStep{
			{Config: cfg("Scottish Gaelic", true), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("optimizelycmssaas_locale.t", "display_name", "Scottish Gaelic"),
				resource.TestCheckResourceAttr("optimizelycmssaas_locale.t", "is_enabled", "true"))},
			keyedImport("optimizelycmssaas_locale.t"),
			{Config: cfg("Scottish Gaelic (TfAcc)", false), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("optimizelycmssaas_locale.t", "display_name", "Scottish Gaelic (TfAcc)"),
				resource.TestCheckResourceAttr("optimizelycmssaas_locale.t", "is_enabled", "false"))},
			{Config: cfg("Scottish Gaelic (TfAcc)", false) + `
data "optimizelycmssaas_locale" "d" { key = optimizelycmssaas_locale.t.key }
output "route" { value = data.optimizelycmssaas_locale.d.route_segment }`,
				Check: resource.TestCheckOutput("route", "gd")},
		},
	})
}

func TestAccPropertyGroup(t *testing.T) {
	cfg := func(name string, order int) string {
		return fmt.Sprintf(`
resource "optimizelycmssaas_property_group" "t" {
  key          = "TfAccGroup"
  display_name = %q
  sort_order   = %d
}`, name, order)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: destroyed("optimizelycmssaas_property_group", func(ctx context.Context, c *client.Client, k string) error {
			_, err := c.PropertyGroups.Get(ctx, k)
			return err
		}),
		Steps: []resource.TestStep{
			{Config: cfg("TfAcc group", 900), Check: resource.TestCheckResourceAttr("optimizelycmssaas_property_group.t", "sort_order", "900")},
			keyedImport("optimizelycmssaas_property_group.t"),
			{Config: cfg("TfAcc group renamed", 901), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("optimizelycmssaas_property_group.t", "display_name", "TfAcc group renamed"),
				resource.TestCheckResourceAttr("optimizelycmssaas_property_group.t", "sort_order", "901"))},
		},
	})
}

func TestAccContentTypeWithTemplateAndBlueprint(t *testing.T) {
	typeCfg := func(display, props string, allowLoss bool) string {
		return fmt.Sprintf(`
resource "optimizelycmssaas_content_type" "t" {
  key             = "TfAccType"
  base_type       = "_component"
  display_name    = %q
  properties_json = jsonencode(%s)
  allow_data_loss = %t
}`, display, props, allowLoss)
	}
	one := `{ title = { type = "string", displayName = "Title" } }`
	two := `{ title = { type = "string", displayName = "Title" }, teaser = { type = "string", displayName = "Teaser" } }`
	template := func(choices string) string {
		return fmt.Sprintf(`
resource "optimizelycmssaas_display_template" "t" {
  key          = "TfAccTemplate"
  display_name = "TfAcc template"
  content_type = optimizelycmssaas_content_type.t.key
  settings = {
    width = {
      display_name = "Width"
      editor       = "select"
      sort_order   = 10
      choices      = %s
    }
  }
}`, choices)
	}
	bothChoices := `{ narrow = { display_name = "Narrow", sort_order = 10 }, wide = { display_name = "Wide", sort_order = 20 } }`
	oneChoice := `{ narrow = { display_name = "Narrow", sort_order = 10 } }`
	blueprint := `
resource "optimizelycmssaas_blueprint" "t" {
  display_name = "TfAcc blueprint"
  content_type = optimizelycmssaas_content_type.t.key
  content_json = jsonencode({ properties = { title = { value = "From blueprint" } } })
}`

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: resource.ComposeTestCheckFunc(
			destroyed("optimizelycmssaas_display_template", func(ctx context.Context, c *client.Client, k string) error {
				_, err := c.DisplayTemplates.Get(ctx, k)
				return err
			}),
			destroyed("optimizelycmssaas_blueprint", func(ctx context.Context, c *client.Client, k string) error {
				_, err := c.Blueprints.Get(ctx, k)
				return err
			}),
			destroyed("optimizelycmssaas_content_type", func(ctx context.Context, c *client.Client, k string) error {
				_, err := c.ContentTypes.Get(ctx, k)
				return err
			}),
		),
		Steps: []resource.TestStep{
			{Config: typeCfg("TfAcc type", one, false) + template(bothChoices) + blueprint, Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("optimizelycmssaas_content_type.t", "base_type", "_component"),
				resource.TestCheckResourceAttr("optimizelycmssaas_display_template.t", "settings.width.choices.%", "2"),
				resource.TestCheckResourceAttrSet("optimizelycmssaas_blueprint.t", "key"))},
			// properties_json is re-serialised from the API on import, so skip the textual compare.
			keyedImport("optimizelycmssaas_content_type.t", "properties_json"),
			keyedImport("optimizelycmssaas_display_template.t"),
			// Adding a property and dropping a choice exercises merge-patch nulls.
			{Config: typeCfg("TfAcc type v2", two, false) + template(oneChoice) + blueprint, Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("optimizelycmssaas_content_type.t", "display_name", "TfAcc type v2"),
				resource.TestCheckResourceAttr("optimizelycmssaas_display_template.t", "settings.width.choices.%", "1"))},
			// Removing a property is a breaking change: refused unless allow_data_loss is set.
			{Config: typeCfg("TfAcc type v2", one, false) + template(oneChoice) + blueprint, ExpectError: regexp.MustCompile(`(?i)data loss`)},
			// With the opt-in it is sent as null and then shows no drift.
			{Config: typeCfg("TfAcc type v2", one, true) + template(oneChoice) + blueprint},
			{Config: typeCfg("TfAcc type v2", one, true) + template(oneChoice) + blueprint, PlanOnly: true},
		},
	})
}

func TestAccApplication(t *testing.T) {
	testAccEnv(t, "OPTIMIZELY_CMS_TEST_CONTAINER", "OPTIMIZELY_CMS_TEST_CONTENT_TYPE")
	// Entry points may not overlap, so the test creates its own start page. The entry point
	// is written with "?loc=en", which the API stores without it: this guards that round trip.
	cfg := func(name, host string) string {
		return fmt.Sprintf(`
resource "optimizelycmssaas_content" "page" {
  content_type     = %q
  container        = %q
  locale           = "en"
  display_name     = "TfAcc start page"
  permanent_delete = true
}
resource "optimizelycmssaas_application" "t" {
  key          = "TfAccApp"
  display_name = %q
  type         = "website"
  entry_point  = "cms://content/${optimizelycmssaas_content.page.key}?loc=en"
  hosts = [{ authority = %q, type = "primary", preferred_url_scheme = "https" }]
}`, os.Getenv("OPTIMIZELY_CMS_TEST_CONTENT_TYPE"), os.Getenv("OPTIMIZELY_CMS_TEST_CONTAINER"), name, host)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: destroyed("optimizelycmssaas_application", func(ctx context.Context, c *client.Client, k string) error {
			_, err := c.Applications.Get(ctx, k)
			return err
		}),
		Steps: []resource.TestStep{
			{Config: cfg("TfAcc app", "tfacc.example.com"), Check: resource.TestCheckResourceAttr("optimizelycmssaas_application.t", "hosts.0.authority", "tfacc.example.com")},
			keyedImport("optimizelycmssaas_application.t", "entry_point"), // import adopts the API's form (no ?loc)
			{Config: cfg("TfAcc app", "tfacc2.example.com"), Check: resource.TestCheckResourceAttr("optimizelycmssaas_application.t", "hosts.0.authority", "tfacc2.example.com")},
			{Config: cfg("TfAcc app", "tfacc2.example.com"), PlanOnly: true}, // no drift despite the dropped ?loc
		},
	})
}

func TestAccContent(t *testing.T) {
	testAccEnv(t, "OPTIMIZELY_CMS_TEST_CONTAINER", "OPTIMIZELY_CMS_TEST_CONTENT_TYPE")
	cfg := func(name string, publish bool) string {
		return fmt.Sprintf(`
resource "optimizelycmssaas_content" "t" {
  content_type     = %q
  container        = %q
  locale           = "en"
  display_name     = %q
  publish          = %t
  permanent_delete = true
}`, os.Getenv("OPTIMIZELY_CMS_TEST_CONTENT_TYPE"), os.Getenv("OPTIMIZELY_CMS_TEST_CONTAINER"), name, publish)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: destroyed("optimizelycmssaas_content", func(ctx context.Context, c *client.Client, k string) error {
			n, err := c.GetContent(ctx, k)
			if err == nil && n.Deleted != nil {
				return client.ErrNotFound
			}
			return err
		}),
		Steps: []resource.TestStep{
			{Config: cfg("TfAcc content", false), Check: resource.TestCheckResourceAttr("optimizelycmssaas_content.t", "status", "draft")},
			{Config: cfg("TfAcc content", true), Check: resource.TestCheckResourceAttr("optimizelycmssaas_content.t", "status", "published")},
			// Editing a published version creates a new draft version.
			{Config: cfg("TfAcc content edited", false), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("optimizelycmssaas_content.t", "display_name", "TfAcc content edited"),
				resource.TestCheckResourceAttr("optimizelycmssaas_content.t", "status", "draft"))},
		},
	})
}

// remoteColumns counts the column nodes of the managed version's composition as the CMS holds it.
func remoteColumns(name string, want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not in state", name)
		}
		c, err := testAccClient()
		if err != nil {
			return err
		}
		v, err := c.GetVersion(context.Background(), rs.Primary.Attributes["key"], rs.Primary.Attributes["version"])
		if err != nil {
			return err
		}
		var root map[string]any
		if err := json.Unmarshal(v.Composition, &root); err != nil {
			return fmt.Errorf("composition not readable: %w", err)
		}
		var count func(n map[string]any) int
		count = func(n map[string]any) int {
			total := 0
			if n["nodeType"] == "column" {
				total++
			}
			kids, _ := n["nodes"].([]any)
			for _, k := range kids {
				if m, ok := k.(map[string]any); ok {
					total += count(m)
				}
			}
			return total
		}
		if got := count(root); got != want {
			return fmt.Errorf("the CMS has %d columns, want %d", got, want)
		}
		return nil
	}
}

// remoteCompositionHas asserts the managed version's composition, as the CMS holds it, contains text.
func remoteCompositionHas(name, text string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[name]
		c, err := testAccClient()
		if err != nil {
			return err
		}
		v, err := c.GetVersion(context.Background(), rs.Primary.Attributes["key"], rs.Primary.Attributes["version"])
		if err != nil {
			return err
		}
		if !strings.Contains(string(v.Composition), text) {
			return fmt.Errorf("the CMS composition does not contain %q", text)
		}
		return nil
	}
}

// TestAccContentComposition covers an experience with one section, one row and two columns, each holding a
// component: create, no drift, edit, node removal, and a new draft once published. A composition change is
// always written as a new version: PATCHing a version's composition does not apply to nodes matched by id.
func TestAccContentComposition(t *testing.T) {
	testAccEnv(t, "OPTIMIZELY_CMS_TEST_CONTAINER", "OPTIMIZELY_CMS_TEST_ELEMENT_TYPE")
	experienceType := os.Getenv("OPTIMIZELY_CMS_TEST_EXPERIENCE_TYPE")
	if experienceType == "" {
		experienceType = "BlankExperience"
	}
	// columns lists the display name of each column's component; one entry per column.
	cfg := func(publish bool, components ...string) string {
		cols := ""
		for _, name := range components {
			cols += fmt.Sprintf(`
          {
            nodeType = "column"
            nodes = [{
              nodeType    = "component"
              displayName = %q
              component   = { contentType = %q }
            }]
          },`, name, os.Getenv("OPTIMIZELY_CMS_TEST_ELEMENT_TYPE"))
		}
		return fmt.Sprintf(`
resource "optimizelycmssaas_content" "t" {
  content_type     = %q
  container        = %q
  locale           = "en"
  display_name     = "TfAcc experience"
  publish          = %t
  permanent_delete = true

  composition_json = jsonencode({
    nodeType   = "experience"
    layoutType = "outline"
    nodes = [{
      nodeType   = "section"
      layoutType = "grid"
      component  = { contentType = "BlankSection" }
      nodes = [{
        nodeType = "row"
        nodes = [%s
        ]
      }]
    }]
  })
}`, experienceType, os.Getenv("OPTIMIZELY_CMS_TEST_CONTAINER"), publish, cols)
	}
	const name = "optimizelycmssaas_content.t"
	var before string
	remember := func(s *terraform.State) error {
		before = s.RootModule().Resources[name].Primary.Attributes["version"]
		return nil
	}
	newVersion := func(s *terraform.State) error {
		if got := s.RootModule().Resources[name].Primary.Attributes["version"]; got == before {
			return fmt.Errorf("editing a published version must create a new draft, but the version is still %s", got)
		}
		return nil
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProviders,
		CheckDestroy: destroyed("optimizelycmssaas_content", func(ctx context.Context, c *client.Client, k string) error {
			n, err := c.GetContent(ctx, k)
			if err == nil && n.Deleted != nil {
				return client.ErrNotFound
			}
			return err
		}),
		Steps: []resource.TestStep{
			// Create with two columns.
			{Config: cfg(false, "First", "Second"), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(name, "status", "draft"), remoteColumns(name, 2))},
			// The CMS adds node ids; that must not show as drift.
			{Config: cfg(false, "First", "Second"), PlanOnly: true},
			// Editing one value reaches the CMS.
			{Config: cfg(false, "First edited", "Second"), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(name, "status", "draft"), remoteColumns(name, 2), remoteCompositionHas(name, "First edited"))},
			// Removing a column removes it in the CMS.
			{Config: cfg(false, "First edited"), Check: resource.ComposeTestCheckFunc(
				remoteColumns(name, 1), remoteCompositionHas(name, "First edited"))},
			// Publish, then edit: a published version is immutable, so a new draft appears.
			{Config: cfg(true, "First edited"), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(name, "status", "published"), remember)},
			{Config: cfg(false, "First again"), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(name, "status", "draft"), remoteColumns(name, 1),
				remoteCompositionHas(name, "First again"), newVersion)},
			// Import reads the composition back; the configured string and the CMS's (with ids) are
			// structurally equal but not byte-identical, so that attribute is not compared here.
			keyedImport(name, "composition_json", "publish", "permanent_delete"),
		},
	})
}

// Read-only: safe against any instance, so it runs whenever TF_ACC=1.
func TestAccReadOnlyDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{{
			Config: `
data "optimizelycmssaas_property_format" "f" { key = "shortString" }
data "optimizelycmssaas_manifest" "m" { sections = ["locales"] }
output "data_type" { value = data.optimizelycmssaas_property_format.f.data_type }
output "manifest_has_locales" { value = can(jsondecode(data.optimizelycmssaas_manifest.m.json).locales) }`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckOutput("data_type", "string"),
				resource.TestCheckOutput("manifest_has_locales", "true")),
		}},
	})
}

// Read-only: finds the content root. Holds on an empty instance, where it must not depend on any application.
func TestAccContentRoot(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{{
			Config: `
data "optimizelycmssaas_content_root" "r" {}
output "reference_ok" { value = startswith(data.optimizelycmssaas_content_root.r.reference, "cms://content/") }
output "root_key" { value = data.optimizelycmssaas_content_root.r.key }`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckOutput("reference_ok", "true"),
				resource.TestCheckOutput("root_key", client.WellKnownRootKey)),
		}},
	})
}

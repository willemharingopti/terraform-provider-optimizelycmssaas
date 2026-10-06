package provider

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/willemharingopti/terraform-provider-optimizelycmssaas/internal/client"
)

func raw(m map[string]string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		out[k] = json.RawMessage(v)
	}
	return out
}

func TestPropertiesEquivalent(t *testing.T) {
	remote := raw(map[string]string{
		"title": `{"type":"string","displayName":"Title","sortOrder":0,"indexingType":"searchable"}`,
		"tags":  `{"type":"array","items":{"type":"string"}}`,
	})
	cases := []struct {
		name string
		cfg  string
		want bool
	}{
		{"api-added defaults ignored", `{"title":{"type":"string","displayName":"Title"},"tags":{"type":"array","items":{"type":"string"}}}`, true},
		{"key order irrelevant", `{"tags":{"items":{"type":"string"},"type":"array"},"title":{"displayName":"Title","type":"string"}}`, true},
		{"changed value", `{"title":{"type":"url"},"tags":{"type":"array","items":{"type":"string"}}}`, false},
		{"property removed in config", `{"title":{"type":"string"}}`, false},
		{"property added out of band is drift", `{"title":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}},"x":{"type":"string"}}`, false},
		{"invalid json", `{`, false},
	}
	for _, tc := range cases {
		if got := propertiesEquivalent(tc.cfg, remote); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestAccessRightsRoundTripDefaultsRole(t *testing.T) {
	got := accessRightsFromAPI([]client.SecurityIdentity{{Name: "Admins"}, {Name: "bot", Type: "application"}})
	if len(got) != 2 || got[0].Type.ValueString() != "role" || got[1].Type.ValueString() != "application" {
		t.Errorf("unexpected: %+v", got)
	}
	if accessRightsFromAPI(nil) != nil {
		t.Error("empty access rights should map to nil")
	}
}

func TestPatchHelpers(t *testing.T) {
	if patchList(nil) != nil || patchList([]string{}) != nil {
		t.Error("empty list should patch as null")
	}
	if v, ok := patchList([]string{"a"}).([]string); !ok || len(v) != 1 {
		t.Error("non-empty list should pass through")
	}
}

func TestSettingsPatchNullsRemovedEntries(t *testing.T) {
	mk := func(choices ...string) displaySettingModel {
		m := displaySettingModel{DisplayName: types.StringValue("S"), SortOrder: types.Int64Value(1), Choices: map[string]displayChoiceModel{}}
		for _, c := range choices {
			m.Choices[c] = displayChoiceModel{DisplayName: types.StringValue(c), SortOrder: types.Int64Value(0)}
		}
		return m
	}
	state := map[string]displaySettingModel{"keep": mk("a", "b"), "gone": mk("x")}
	plan := map[string]displaySettingModel{"keep": mk("a")}
	b, _ := json.Marshal(settingsPatch(plan, state))
	want := `{"gone":null,"keep":{"choices":{"a":{"displayName":"a","sortOrder":0},"b":null},"displayName":"S","editor":null,"sortOrder":1}}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	if settingsPatch(nil, nil) != nil {
		t.Error("no settings should patch as null")
	}
}

func TestPreviewFormatsPatch(t *testing.T) {
	b, _ := json.Marshal(previewFormatsPatch(map[string]string{"any": "x"}, map[string]string{"any": "old", "_page": "y"}))
	if string(b) != `{"_page":null,"any":"x"}` {
		t.Errorf("got %s", b)
	}
	if previewFormatsPatch(nil, nil) != nil {
		t.Error("nothing to patch should be nil")
	}
}

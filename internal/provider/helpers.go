package provider

import (
	"encoding/json"
	"reflect"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---- null / empty handling ----
// The API omits or empties unset values; Terraform distinguishes null from "".
// Optional (non-computed) attributes therefore map "" / empty list <-> null.

func strOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// patchStr is the merge-patch value for an optional string: null clears it.
func patchStr(v types.String) any {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return v.ValueString()
}

func patchList(l []string) any {
	if len(l) == 0 {
		return nil
	}
	return l
}

func listOrNil(l []string) []string {
	if len(l) == 0 {
		return nil
	}
	return l
}

func int64Ptr(v types.Int64) *int32 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := int32(v.ValueInt64())
	return &i
}

func int32Val(p *int32) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func boolVal(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}

// ---- access rights (shared by content types and locales) ----

type accessRightModel struct {
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

func accessRightsAttribute(desc string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional:    true,
		Description: desc,
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true, Description: "Name of the user, role or application."},
			"type": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("role"),
				Description: "user, role or application. Defaults to role.",
			},
		}},
	}
}

func accessRightsToAPI(in []accessRightModel) []client.SecurityIdentity {
	var out []client.SecurityIdentity
	for _, a := range in {
		out = append(out, client.SecurityIdentity{Name: a.Name.ValueString(), Type: a.Type.ValueString()})
	}
	return out
}

func accessRightsFromAPI(in []client.SecurityIdentity) []accessRightModel {
	var out []accessRightModel
	for _, a := range in {
		t := a.Type
		if t == "" {
			t = "role"
		}
		out = append(out, accessRightModel{Name: types.StringValue(a.Name), Type: types.StringValue(t)})
	}
	return out
}

func patchAccessRights(in []accessRightModel) any {
	if len(in) == 0 {
		return nil
	}
	return accessRightsToAPI(in)
}

// ---- semantic JSON comparison ----

// propertiesEquivalent reports whether the user's configured properties JSON
// describes the same properties as the API returned. Extra fields the API adds
// to a property (defaults) are tolerated; missing/extra properties are not.
func propertiesEquivalent(configured string, remote map[string]json.RawMessage) bool {
	var want map[string]any
	if err := json.Unmarshal([]byte(configured), &want); err != nil {
		return false
	}
	if len(want) != len(remote) {
		return false
	}
	for k, raw := range remote {
		w, ok := want[k]
		if !ok {
			return false
		}
		var got any
		if err := json.Unmarshal(raw, &got); err != nil || !jsonSubset(w, got) {
			return false
		}
	}
	return true
}

// jsonSubset reports whether every field in want is present and equal in got.
func jsonSubset(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			if gv, ok := g[k]; !ok || !jsonSubset(wv, gv) {
				return false
			}
		}
		return true
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !jsonSubset(w[i], g[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(want, got)
	}
}

// mergePatchDiff returns the minimal RFC 7396 merge-patch turning old into new:
// changed values replace, keys missing from new become null.
func mergePatchDiff(oldV, newV map[string]any) map[string]any {
	patch := map[string]any{}
	for k, nv := range newV {
		ov, ok := oldV[k]
		if !ok {
			patch[k] = nv
			continue
		}
		nm, nIsMap := nv.(map[string]any)
		om, oIsMap := ov.(map[string]any)
		if nIsMap && oIsMap {
			if sub := mergePatchDiff(om, nm); len(sub) > 0 {
				patch[k] = sub
			}
		} else if !reflect.DeepEqual(ov, nv) {
			patch[k] = nv
		}
	}
	for k := range oldV {
		if _, ok := newV[k]; !ok {
			patch[k] = nil
		}
	}
	return patch
}

// parseObject parses a JSON object string; null/empty yields nil.
func parseObject(s types.String) (map[string]any, error) {
	if s.IsNull() || s.IsUnknown() || s.ValueString() == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s.ValueString()), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// reconcileJSON keeps the configured JSON string when it is a structural subset
// of what the API returned (so API-added defaults do not cause drift) and
// otherwise adopts the remote value.
func reconcileJSON(prior types.String, remote any) types.String {
	empty := remote == nil
	if m, ok := remote.(map[string]any); ok {
		empty = len(m) == 0
	}
	if (prior.IsNull() || prior.IsUnknown() || prior.ValueString() == "") && empty {
		return types.StringNull()
	}
	if !prior.IsNull() && !prior.IsUnknown() {
		var want any
		if json.Unmarshal([]byte(prior.ValueString()), &want) == nil && jsonSubset(want, remote) {
			return prior
		}
	}
	b, _ := json.Marshal(remote)
	return types.StringValue(string(b))
}

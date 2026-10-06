package provider

import "testing"

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Main site":           "main_site",
		"  Café Main_Site!! ": "cafe_main_site",
		"already-a-slug":      "already_a_slug",
		"A  &  B 2":           "a_b_2",
		"2nd site":            "_2nd_site",
		"!!!":                 "",
		"":                    "",
	} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

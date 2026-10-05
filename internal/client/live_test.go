package client

import (
	"context"
	"os"
	"testing"
)

// TestLiveReadOnly exercises the real API with GET requests only. It is skipped
// unless OPTIMIZELY_CMS_CLIENT_ID and OPTIMIZELY_CMS_CLIENT_SECRET are set.
func TestLiveReadOnly(t *testing.T) {
	id, secret := os.Getenv("OPTIMIZELY_CMS_CLIENT_ID"), os.Getenv("OPTIMIZELY_CMS_CLIENT_SECRET")
	if id == "" || secret == "" {
		t.Skip("credentials not set")
	}
	c, err := New(os.Getenv("OPTIMIZELY_CMS_BASE_URL"), os.Getenv("OPTIMIZELY_CMS_TOKEN_URL"), id, secret)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, p := range []string{"/locales", "/contenttypes", "/contentsources", "/contenttypebindings", "/displaytemplates", "/applications", "/propertygroups", "/propertyformats", "/blueprints"} {
		var page struct {
			Items []map[string]any `json:"items"`
		}
		if err := c.Do(ctx, "GET", p+"?pageSize=5", "", nil, &page); err != nil {
			t.Errorf("GET %s: %v", p, err)
			continue
		}
		t.Logf("GET %s -> %d items", p, len(page.Items))
	}
	if _, err := c.Locales.Get(ctx, "does-not-exist-zz"); err == nil {
		t.Error("expected not found")
	}
}

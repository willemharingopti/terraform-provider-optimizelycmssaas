package client

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestCreateContentDecodesEmbeddedNodeAndVersion(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"k1","contentType":"Article","primaryLocale":"en","initialVersion":{"version":"v1","status":"draft","displayName":"Hi","locale":"en"}}`))
	}
	got, err := c.CreateContent(context.Background(), &NewContent{ContentType: "Article", InitialVersion: ContentVersion{DisplayName: "Hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "k1" || got.InitialVersion.Version != "v1" || got.PrimaryLocale != "en" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestPublishUsesResponseBody(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.Header.Get("Prefer") == "return=representation" {
			_, _ = w.Write([]byte(`{"version":"v1","status":"published"}`))
			return
		}
		_, _ = w.Write([]byte(`{"version":"v1","status":"draft"}`)) // stale read
	}
	v, err := c.PublishVersion(context.Background(), "k1", "v1")
	if err != nil || v.Status != "published" {
		t.Fatalf("want the publish response: %v %+v", err, v)
	}
}

func TestPublishPollsWhenNoBodyAndReadsAreStale(t *testing.T) {
	f, c := newFake(t)
	gets := 0
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if gets++; gets < 3 {
			_, _ = w.Write([]byte(`{"version":"v1","status":"draft"}`))
			return
		}
		_, _ = w.Write([]byte(`{"version":"v1","status":"published"}`))
	}
	v, err := c.PublishVersion(context.Background(), "k1", "v1")
	if err != nil || v.Status != "published" || gets != 3 {
		t.Fatalf("%v %+v gets=%d", err, v, gets)
	}
}

func TestPublishUsesColonActionAndRereads(t *testing.T) {
	f, c := newFake(t)
	var paths []string
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.EscapedPath())
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"version":"v1","status":"published","displayName":"Hi"}`))
	}
	v, err := c.PublishVersion(context.Background(), "k1", "v1")
	if err != nil || v.Status != "published" {
		t.Fatalf("%v %+v", err, v)
	}
	want := []string{"POST /v1/content/k1/versions/v1:publish", "GET /v1/content/k1/versions/v1"}
	if len(paths) != 2 || paths[0] != want[0] || paths[1] != want[1] {
		t.Errorf("got %v want %v", paths, want)
	}
}

func TestDeleteContentPermanentHeader(t *testing.T) {
	var header string
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete { // the "is it gone?" reads
			w.WriteHeader(http.StatusNotFound)
			return
		}
		header = r.Header.Get("cms-permanent-delete")
		w.WriteHeader(http.StatusNoContent)
	}
	_ = c.DeleteContent(context.Background(), "k1", true)
	if header != "true" {
		t.Errorf("permanent header = %q", header)
	}
	_ = c.DeleteContent(context.Background(), "k1", false)
	if header != "" {
		t.Errorf("soft delete must not send the header, got %q", header)
	}
}

func TestListVersionsQuery(t *testing.T) {
	f, c := newFake(t)
	var q string
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[{"version":"a","displayName":"x"},{"version":"b","displayName":"y"}]}`))
	}
	vs, err := c.ListVersions(context.Background(), "k1", "sv-SE")
	if err != nil || len(vs) != 2 {
		t.Fatalf("%v %v", err, vs)
	}
	if q != "locales=sv-SE&pageSize=100" {
		t.Errorf("query = %s", q)
	}
}

func TestExportManifestReturnsRaw(t *testing.T) {
	f, c := newFake(t)
	var q string
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"locales":[]}`))
	}
	raw, err := c.ExportManifest(context.Background(), []string{"locales", "contentTypes"}, true)
	if err != nil || string(raw) != `{"locales":[]}` {
		t.Fatalf("%v %s", err, raw)
	}
	if q != "includeReadOnly=true&sections=locales%2CcontentTypes" {
		t.Errorf("query = %s", q)
	}
}

func TestContentKeyFromRef(t *testing.T) {
	cases := map[string]struct {
		key string
		ok  bool
	}{
		"cms://content/abc123":               {"abc123", true},
		"cms://content/abc123?loc=en&ver=2":  {"abc123", true},
		"newsite":                            {"", false},
		"cms://content/":                     {"", false},
		"https://example.com/cms://content/": {"", false},
	}
	for in, want := range cases {
		if k, ok := ContentKeyFromRef(in); k != want.key || ok != want.ok {
			t.Errorf("%q: got (%q,%v) want (%q,%v)", in, k, ok, want.key, want.ok)
		}
	}
}

func TestFindRootWalksUpToItemWithoutContainer(t *testing.T) {
	f, c := newFake(t)
	parents := map[string]string{"page": "start", "start": "root", "root": ""}
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[len("/v1/content/"):]
		p, ok := parents[key]
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"key":"` + key + `","container":"` + p + `"}`))
	}
	root, hops, err := c.FindRoot(context.Background(), "page")
	if err != nil || root != "root" || hops != 2 {
		t.Fatalf("got %q after %d hops, err %v", root, hops, err)
	}
	if _, _, err := c.FindRoot(context.Background(), "unreadable"); err == nil {
		t.Error("an unreadable start must be an error")
	}
}

func TestFindRootStopsOnCycles(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[len("/v1/content/"):]
		next := map[string]string{"a": "b", "b": "a"}[key]
		_, _ = w.Write([]byte(`{"key":"` + key + `","container":"` + next + `"}`))
	}
	if _, _, err := c.FindRoot(context.Background(), "a"); err == nil {
		t.Fatal("a cyclic hierarchy must not loop forever")
	}
}

func TestListFollowsPages(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		items := ""
		n := 100
		if r.URL.Query().Get("pageIndex") == "1" {
			n = 3
		}
		for i := 0; i < n; i++ {
			if i > 0 {
				items += ","
			}
			items += `{"key":"k"}`
		}
		_, _ = w.Write([]byte(`{"items":[` + items + `]}`))
	}
	got, err := c.Applications.List(context.Background())
	if err != nil || len(got) != 103 {
		t.Fatalf("got %d items, err %v", len(got), err)
	}
}

func TestWellKnownRoot(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/content/"+WellKnownRootKey {
			_, _ = w.Write([]byte(`{"key":"` + WellKnownRootKey + `","locales":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
	if k, ok := c.ConfiguredRoot(context.Background()); !ok || k != WellKnownRootKey {
		t.Fatalf("got %q %v", k, ok)
	}
	// An item with that key but a parent is not a root; absence is not an error.
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"key":"` + WellKnownRootKey + `","container":"x"}`))
	}
	if _, ok := c.ConfiguredRoot(context.Background()); ok {
		t.Error("an item with a container must not be treated as the root")
	}
	f.handle = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }
	if _, ok := c.ConfiguredRoot(context.Background()); ok {
		t.Error("an unreadable root must fall through to the walk-up, not succeed")
	}
}

func TestDeleteContentRetriesWhileEntryPointRefusalLingers(t *testing.T) {
	f, c := newFake(t)
	deletes := 0
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			if deletes++; deletes < 3 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"detail":"Cannot delete content because it is set as entry point for the 'App' application."}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
	if err := c.DeleteContent(context.Background(), "k1", true); err != nil {
		t.Fatal(err)
	}
	if deletes != 3 {
		t.Errorf("want 3 delete attempts, got %d", deletes)
	}
}

func TestDeleteContentDoesNotRetryOtherErrorsAndGivesUpEventually(t *testing.T) {
	old := visibilityWait
	visibilityWait = 400 * time.Millisecond
	defer func() { visibilityWait = old }()
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"nope"}`))
	}
	start := time.Now()
	if err := c.DeleteContent(context.Background(), "k1", false); err == nil || time.Since(start) > 200*time.Millisecond {
		t.Errorf("an unrelated 400 must fail immediately, got %v after %s", err, time.Since(start))
	}
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"... set as entry point for the 'App' application."}`))
	}
	if err := c.DeleteContent(context.Background(), "k1", false); err == nil {
		t.Error("a persistent refusal must surface after the timeout")
	}
}

func TestDeleteWaitsUntilGone(t *testing.T) {
	f, c := newFake(t)
	gets := 0
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if gets++; gets < 3 { // replica still serves the deleted item
			_, _ = w.Write([]byte(`{"key":"Grp"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
	if err := c.PropertyGroups.Delete(context.Background(), "Grp"); err != nil {
		t.Fatal(err)
	}
	if gets != 2+settledReads {
		t.Errorf("got %d reads, want 2 stale + %d settled 404s", gets, settledReads)
	}
}

func TestNormalizeRootKey(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"43f936c99b234ea397b261c538ad07c9":     {"43f936c99b234ea397b261c538ad07c9", true},
		"43f936c9-9b23-4ea3-97b2-61c538ad07c9": {"43f936c99b234ea397b261c538ad07c9", true}, // dashed form, which the API rejects
		"  43F936C99B234EA397B261C538AD07C9 ":  {"43f936c99b234ea397b261c538ad07c9", true},
		"../admin":                             {"", false},
		"a/b":                                  {"", false},
		"a?b=1":                                {"", false},
	}
	for in, want := range cases {
		got, err := normalizeRootKey(in)
		if (err == nil) != want.ok || got != want.want {
			t.Errorf("%q: got (%q, %v), want (%q, ok=%v)", in, got, err, want.want, want.ok)
		}
	}
}

func TestConfiguredRootKeyIsUsedAndReportedAsExplicit(t *testing.T) {
	f, _ := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/content/abc123" {
			_, _ = w.Write([]byte(`{"key":"abc123","locales":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
	c, err := New(f.srv.URL, "", "id", "secret", WithHTTPClient(f.srv.Client()), WithRootKey("abc123"))
	if err != nil {
		t.Fatal(err)
	}
	if k, explicit := c.RootKey(); k != "abc123" || !explicit {
		t.Errorf("RootKey = %q explicit=%v", k, explicit)
	}
	if k, ok := c.ConfiguredRoot(context.Background()); !ok || k != "abc123" {
		t.Errorf("got %q %v", k, ok)
	}
	d, _ := New(f.srv.URL, "", "id", "secret", WithHTTPClient(f.srv.Client()))
	if k, explicit := d.RootKey(); k != WellKnownRootKey || explicit {
		t.Errorf("default RootKey = %q explicit=%v", k, explicit)
	}
	if _, err := New(f.srv.URL, "", "id", "secret", WithRootKey("../x")); err == nil {
		t.Error("an invalid root key must be refused")
	}
}

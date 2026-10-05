package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fake struct {
	srv    *httptest.Server
	tokens atomic.Int32
	hits   atomic.Int32
	last   struct{ method, path, ctype, auth, body string }
	handle func(w http.ResponseWriter, r *http.Request)
}

func newFake(t *testing.T) (*fake, *Client) {
	t.Helper()
	f := &fake{}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			f.tokens.Add(1)
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["grant_type"] != "client_credentials" || in["client_id"] != "id" || in["client_secret"] != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":300,"token_type":"Bearer"}`))
			return
		}
		f.hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		f.last.method, f.last.path, f.last.ctype, f.last.auth, f.last.body =
			r.Method, r.URL.EscapedPath(), r.Header.Get("Content-Type"), r.Header.Get("Authorization"), string(b)
		f.handle(w, r)
	}))
	t.Cleanup(f.srv.Close)
	c, err := New(f.srv.URL, "", "id", "secret", WithHTTPClient(f.srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}

func TestNewValidation(t *testing.T) {
	if _, err := New("http://insecure.example", "", "id", "s"); err == nil {
		t.Error("expected http base URL to be rejected")
	}
	if _, err := New("https://ok.example", "http://insecure.example/token", "id", "s"); err == nil {
		t.Error("expected http token URL to be rejected")
	}
	if _, err := New("https://ok.example", "", "", ""); err == nil {
		t.Error("expected missing credentials to be rejected")
	}
	c, err := New("https://ok.example/v1/", "", "id", "s")
	if err != nil || c.baseURL != "https://ok.example/v1" || c.tokenURL != "https://ok.example/oauth/token" {
		t.Errorf("unexpected normalisation: %+v %v", c, err)
	}
}

func TestGetUsesBearerAndCachesToken(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"key":"en","displayName":"English","routeSegment":"en"}`))
	}
	for i := 0; i < 3; i++ {
		l, err := c.Locales.Get(context.Background(), "en")
		if err != nil || l.DisplayName != "English" {
			t.Fatalf("get: %v %+v", err, l)
		}
	}
	if f.tokens.Load() != 1 {
		t.Errorf("token fetched %d times, want 1", f.tokens.Load())
	}
	if f.last.auth != "Bearer tok" || f.last.path != "/v1/locales/en" {
		t.Errorf("unexpected request: %+v", f.last)
	}
}

func TestKeyIsPathEscaped(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }
	_, _ = c.Locales.Get(context.Background(), "../contenttypes/x?y")
	if f.last.path != "/v1/locales/..%2Fcontenttypes%2Fx%3Fy" {
		t.Errorf("key not escaped: %s", f.last.path)
	}
}

func TestNotFound(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }
	if _, err := c.Locales.Get(context.Background(), "xx"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestPatchUsesResponseBodyInsteadOfStaleReread(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			if r.Header.Get("Prefer") != "return=representation" || r.Header.Get("X-Extra") != "1" {
				t.Errorf("missing headers: %v", r.Header)
			}
			_, _ = w.Write([]byte(`{"key":"en","displayName":"Fresh","routeSegment":"en"}`))
			return
		}
		_, _ = w.Write([]byte(`{"key":"en","displayName":"STALE","routeSegment":"en"}`))
	}
	l, err := c.Locales.PatchH(context.Background(), "en", map[string]any{"displayName": "Fresh"}, map[string]string{"X-Extra": "1"})
	if err != nil || l.DisplayName != "Fresh" {
		t.Fatalf("want the PATCH response, not the stale GET: %v %+v", err, l)
	}
}

func TestPatchUsesMergePatchAndRereads(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"key":"en","displayName":"New","routeSegment":"en"}`))
	}
	l, err := c.Locales.Patch(context.Background(), "en", map[string]any{"displayName": "New", "fallback": nil})
	if err != nil || l.DisplayName != "New" {
		t.Fatalf("patch: %v %+v", err, l)
	}
	if f.hits.Load() < 2 {
		t.Errorf("a PATCH answered with 204 must be followed by reads, got %d requests", f.hits.Load())
	}
}

func TestPatchContentType(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"key":"a","displayName":"A"}`))
	}
	var sent string
	h := f.handle
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			sent = f.last.ctype + "|" + f.last.body
		}
		h(w, r)
	}
	_, err := c.ContentTypes.Patch(context.Background(), "a", map[string]any{"properties": map[string]any{"gone": nil}})
	if err != nil {
		t.Fatal(err)
	}
	if sent != `application/merge-patch+json|{"properties":{"gone":null}}` {
		t.Errorf("unexpected patch: %s", sent)
	}
}

func TestRetriesOn429(t *testing.T) {
	f, c := newFake(t)
	var n atomic.Int32
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"key":"en"}`))
	}
	if _, err := c.Locales.Get(context.Background(), "en"); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 2 {
		t.Errorf("want 2 attempts, got %d", n.Load())
	}
}

func TestRefreshesTokenOnceOn401(t *testing.T) {
	f, c := newFake(t)
	var n atomic.Int32
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"key":"en"}`))
	}
	if _, err := c.Locales.Get(context.Background(), "en"); err != nil {
		t.Fatal(err)
	}
	if f.tokens.Load() != 2 {
		t.Errorf("want token refresh, got %d token requests", f.tokens.Load())
	}
}

func TestBadCredentialsDoNotLeakSecret(t *testing.T) {
	f, _ := newFake(t)
	c, _ := New(f.srv.URL, "", "id", "wrong-secret", WithHTTPClient(f.srv.Client()))
	_, err := c.Locales.Get(context.Background(), "en")
	if err == nil || contains(err.Error(), "wrong-secret") {
		t.Errorf("error should exist and not contain the secret: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestCreateAsksForRepresentationAndFallsBackToGet(t *testing.T) {
	f, c := newFake(t)
	var prefer string
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			prefer = r.Header.Get("Prefer")
			w.WriteHeader(http.StatusCreated) // empty body, as the live API did without Prefer
			return
		}
		_, _ = w.Write([]byte(`{"key":"Grp","displayName":"Group"}`))
	}
	got, err := c.PropertyGroups.Create(context.Background(), "Grp", &PropertyGroup{Key: "Grp", DisplayName: "Group"})
	if err != nil || got.Key != "Grp" || got.DisplayName != "Group" {
		t.Fatalf("empty body should fall back to GET: %v %+v", err, got)
	}
	if prefer != "return=representation" {
		t.Errorf("Prefer header = %q", prefer)
	}
	if _, err := c.Blueprints.Create(context.Background(), "", &Blueprint{DisplayName: "x"}); err == nil {
		t.Error("empty body with unknown key must be an error, not a zero value")
	}
}

func TestEmptyKeyNeverReachesTheCollectionEndpoint(t *testing.T) {
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}
	ctx := context.Background()
	if _, err := c.Locales.Get(ctx, ""); err == nil {
		t.Error("Get with empty key must fail")
	}
	if err := c.Locales.Delete(ctx, ""); err == nil {
		t.Error("Delete with empty key must fail")
	}
	if _, err := c.Locales.Patch(ctx, "", map[string]any{}); err == nil {
		t.Error("Patch with empty key must fail")
	}
}

func TestCreateWaitsUntilTheNewResourceIsReadable(t *testing.T) {
	f, c := newFake(t)
	gets := 0
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"key":"Grp","displayName":"Group"}`))
			return
		}
		if gets++; gets < 3 { // the API cannot read it back yet
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"key":"Grp","displayName":"Group"}`))
	}
	if _, err := c.PropertyGroups.Create(context.Background(), "Grp", &PropertyGroup{Key: "Grp", DisplayName: "Group"}); err != nil {
		t.Fatal(err)
	}
	if gets != 2+settledReads {
		t.Errorf("create returned after %d reads, want 2 misses then %d settled reads", gets, settledReads)
	}
}

func TestCreateDoesNotHangWhenNeverReadable(t *testing.T) {
	old := visibilityWait
	visibilityWait = 300 * time.Millisecond
	defer func() { visibilityWait = old }()
	f, c := newFake(t)
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"key":"Grp","displayName":"Group"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
	start := time.Now()
	got, err := c.PropertyGroups.Create(context.Background(), "Grp", &PropertyGroup{Key: "Grp", DisplayName: "Group"})
	if err != nil || got.Key != "Grp" {
		t.Fatalf("create must still succeed with the POST representation: %v %+v", err, got)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("waited too long: %s", time.Since(start))
	}
}

func TestWriteWaitsUntilReadsShowTheWrittenVersion(t *testing.T) {
	f, c := newFake(t)
	gets := 0
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			_, _ = w.Write([]byte(`{"key":"en","displayName":"New","lastModified":"2026-10-05T10:00:05+00:00"}`))
			return
		}
		gets++
		lm := "2026-10-05T10:00:01+00:00" // a replica that has not seen the write
		if gets > 4 {
			lm = "2026-10-05T10:00:05+00:00"
		}
		_, _ = w.Write([]byte(`{"key":"en","displayName":"x","lastModified":"` + lm + `"}`))
	}
	if _, err := c.Locales.Patch(context.Background(), "en", map[string]any{"displayName": "New"}); err != nil {
		t.Fatal(err)
	}
	if gets < 4+settledReads {
		t.Errorf("returned after only %d reads; replicas had not caught up until read 5", gets)
	}
}

func TestAPIVersionResolution(t *testing.T) {
	cases := []struct {
		name, host, opt, want string
		wantErr               bool
	}{
		{"default", "https://h.example", "", "https://h.example/v1", false},
		{"version on the host is used", "https://h.example/v2", "", "https://h.example/v2", false},
		{"explicit setting wins over the host", "https://h.example/v1", "v3", "https://h.example/v3", false},
		{"trailing slash tolerated", "https://h.example/v2/", "", "https://h.example/v2", false},
		{"not a version", "https://h.example", "latest", "", true},
		{"path injection refused", "https://h.example", "v1/../admin", "", true},
		{"query injection refused", "https://h.example", "v1?x=1", "", true},
		{"uppercase refused", "https://h.example", "V2", "", true},
		{"empty digits refused", "https://h.example", "v", "", true},
	}
	for _, tc := range cases {
		c, err := New(tc.host, "", "id", "s", WithAPIVersion(tc.opt))
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected an error", tc.name)
			}
			continue
		}
		if err != nil || c.baseURL != tc.want {
			t.Errorf("%s: got %v / %v, want %s", tc.name, c, err, tc.want)
		}
	}
}

func TestRequestsUseTheConfiguredVersion(t *testing.T) {
	f, c0 := newFake(t)
	_ = c0
	c, err := New(f.srv.URL, "", "id", "secret", WithHTTPClient(f.srv.Client()), WithAPIVersion("v2"))
	if err != nil {
		t.Fatal(err)
	}
	f.handle = func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"key":"en"}`)) }
	if _, err := c.Locales.Get(context.Background(), "en"); err != nil {
		t.Fatal(err)
	}
	if f.last.path != "/v2/locales/en" {
		t.Errorf("path = %s, want /v2/locales/en", f.last.path)
	}
}

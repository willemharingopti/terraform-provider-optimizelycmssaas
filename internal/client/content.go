package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type ContentNode struct {
	Key           string   `json:"key,omitempty"`
	LastModified  string   `json:"lastModified,omitempty"`
	Container     string   `json:"container,omitempty"`
	Owner         string   `json:"owner,omitempty"`
	ContentType   string   `json:"contentType,omitempty"`
	PrimaryLocale string   `json:"primaryLocale,omitempty"`
	Locales       []string `json:"locales,omitempty"`
	Deleted       *string  `json:"deleted,omitempty"`
}

// ContentVersion is one locale/version of a content item. Read-only fields
// (version, status, contentType, lastModified) are omitted when empty.
type ContentVersion struct {
	Key          string                     `json:"key,omitempty"`
	Locale       string                     `json:"locale,omitempty"`
	Version      string                     `json:"version,omitempty"`
	ContentType  string                     `json:"contentType,omitempty"`
	DisplayName  string                     `json:"displayName"`
	Status       string                     `json:"status,omitempty"`
	RouteSegment string                     `json:"routeSegment,omitempty"`
	SimpleRoute  string                     `json:"simpleRoute,omitempty"`
	LastModified string                     `json:"lastModified,omitempty"`
	Properties   map[string]json.RawMessage `json:"properties,omitempty"`
}

type NewContent struct {
	Key            string         `json:"key,omitempty"`
	ContentType    string         `json:"contentType"`
	Container      string         `json:"container,omitempty"`
	Owner          string         `json:"owner,omitempty"`
	Blueprint      string         `json:"blueprint,omitempty"`
	InitialVersion ContentVersion `json:"initialVersion"`
}

type NewContentNode struct {
	ContentNode
	InitialVersion ContentVersion `json:"initialVersion"`
}

func contentPath(key string) string { return "/content/" + url.PathEscape(key) }

func versionPath(key, version string) string {
	return contentPath(key) + "/versions/" + url.PathEscape(version)
}

func (c *Client) CreateContent(ctx context.Context, in *NewContent) (*NewContentNode, error) {
	var out NewContentNode
	if err := c.DoH(ctx, http.MethodPost, "/content", "", preferRepresentation, in, &out); err != nil {
		return nil, err
	}
	if out.Key == "" {
		return nil, errors.New("POST /content: the API returned no representation of the created content")
	}
	c.awaitRead(ctx, contentPath(out.Key), out.LastModified)
	return &out, nil
}

func (c *Client) GetContent(ctx context.Context, key string) (*ContentNode, error) {
	var out ContentNode
	if err := c.Do(ctx, http.MethodGet, contentPath(key), "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PatchContent(ctx context.Context, key string, patch map[string]any) (*ContentNode, error) {
	var out ContentNode
	if err := c.DoH(ctx, http.MethodPatch, contentPath(key), "application/merge-patch+json", preferRepresentation, patch, &out); err != nil {
		return nil, err
	}
	if out.Key == "" { // no body: fall back to a read
		c.awaitRead(ctx, contentPath(key), "")
		return c.GetContent(ctx, key)
	}
	c.awaitRead(ctx, contentPath(key), out.LastModified)
	return &out, nil
}

// DeleteContent soft-deletes unless permanent is set. The CMS refuses to delete a page that an
// application still lists as its entry point; right after that application was deleted the
// refusal can linger for a moment, so that one case is retried for a bounded time.
func (c *Client) DeleteContent(ctx context.Context, key string, permanent bool) error {
	var h map[string]string
	if permanent {
		h = map[string]string{"cms-permanent-delete": "true"}
	}
	deadline := time.Now().Add(visibilityWait)
	for {
		err := c.DoH(ctx, http.MethodDelete, contentPath(key), "", h, nil, nil)
		var api *APIError
		if err == nil {
			c.awaitGone(ctx, contentPath(key))
			return nil
		}
		if !errors.As(err, &api) || api.Status != http.StatusBadRequest || !strings.Contains(api.Body, "entry point") || time.Now().After(deadline) {
			return err
		}
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return err
		}
	}
}

func (c *Client) GetVersion(ctx context.Context, key, version string) (*ContentVersion, error) {
	var out ContentVersion
	if err := c.Do(ctx, http.MethodGet, versionPath(key, version), "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListVersions returns up to one page (100) of versions, optionally for one locale.
func (c *Client) ListVersions(ctx context.Context, key, locale string) ([]ContentVersion, error) {
	q := url.Values{"pageSize": {"100"}}
	if locale != "" {
		q.Set("locales", locale)
	}
	var page struct {
		Items []ContentVersion `json:"items"`
	}
	if err := c.Do(ctx, http.MethodGet, contentPath(key)+"/versions?"+q.Encode(), "", nil, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (c *Client) CreateVersion(ctx context.Context, key string, in *ContentVersion) (*ContentVersion, error) {
	var out ContentVersion
	if err := c.DoH(ctx, http.MethodPost, contentPath(key)+"/versions", "", preferRepresentation, in, &out); err != nil {
		return nil, err
	}
	if out.Version != "" {
		c.awaitRead(ctx, versionPath(key, out.Version), out.LastModified)
	}
	return &out, nil
}

func (c *Client) PatchVersion(ctx context.Context, key, version string, patch map[string]any) (*ContentVersion, error) {
	var out ContentVersion
	if err := c.DoH(ctx, http.MethodPatch, versionPath(key, version), "application/merge-patch+json", preferRepresentation, patch, &out); err != nil {
		return nil, err
	}
	if out.Version == "" { // no body: fall back to a read
		c.awaitRead(ctx, versionPath(key, version), "")
		return c.GetVersion(ctx, key, version)
	}
	c.awaitRead(ctx, versionPath(key, version), out.LastModified)
	return &out, nil
}

// PublishVersion publishes immediately and returns the published version. The API
// answers with the version when asked to; if it does not, the version is re-read for a
// short while because a read straight after a write can still show the old status.
func (c *Client) PublishVersion(ctx context.Context, key, version string) (*ContentVersion, error) {
	var out ContentVersion
	if err := c.DoH(ctx, http.MethodPost, versionPath(key, version)+":publish", "", preferRepresentation, map[string]any{}, &out); err != nil {
		return nil, err
	}
	if out.Status != "" {
		c.awaitRead(ctx, versionPath(key, version), out.LastModified)
		return &out, nil
	}
	var last *ContentVersion
	for i := 0; i < 6; i++ {
		v, err := c.GetVersion(ctx, key, version)
		if err != nil {
			return nil, err
		}
		if last = v; v.Status == "published" {
			return v, nil
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return last, nil
}

// ExportManifest returns the raw manifest JSON.
func (c *Client) ExportManifest(ctx context.Context, sections []string, includeReadOnly bool) (json.RawMessage, error) {
	q := url.Values{}
	if len(sections) > 0 {
		q.Set("sections", strings.Join(sections, ","))
	}
	if includeReadOnly {
		q.Set("includeReadOnly", "true")
	}
	p := "/manifest"
	if len(q) > 0 {
		p += "?" + q.Encode()
	}
	var out json.RawMessage
	if err := c.Do(ctx, http.MethodGet, p, "", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ContentKeyFromRef extracts the key from a content reference such as
// cms://content/<key>?loc=en&ver=1.
func ContentKeyFromRef(ref string) (string, bool) {
	const prefix = "cms://content/"
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	key, _, _ := strings.Cut(strings.TrimPrefix(ref, prefix), "?")
	return key, key != ""
}

// WellKnownRootKey is the default key of the built-in root of the content tree. It is not
// documented: it was observed to be identical on separate instances (including an empty one),
// where it exists without a container or locales while arbitrary keys return 404. It can be
// overridden with WithRootKey / OPTIMIZELY_CMS_CONTENT_ROOT_KEY.
const WellKnownRootKey = "43f936c99b234ea397b261c538ad07c9"

// rootKeyRe is deliberately permissive (keys are 32 hex digits today); it only keeps obvious
// garbage out of request paths, which are escaped anyway.
var rootKeyRe = regexp.MustCompile(`^[0-9A-Za-z_]{1,64}$`)

// normalizeRootKey accepts a key as the API wants it (no dashes) or as a dashed GUID
// (43f936c9-9b23-4ea3-97b2-61c538ad07c9), which the API itself rejects with 404.
func normalizeRootKey(k string) (string, error) {
	k = strings.TrimSpace(k)
	if guidRe.MatchString(k) {
		k = strings.ReplaceAll(k, "-", "")
	}
	if !rootKeyRe.MatchString(k) {
		return "", fmt.Errorf("content_root_key %q is invalid: use the content key, e.g. %s", k, WellKnownRootKey)
	}
	return strings.ToLower(k), nil
}

var guidRe = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// RootKey returns the configured root key and whether it was set explicitly.
func (c *Client) RootKey() (key string, explicit bool) { return c.rootKey, c.rootKeyExplicit }

// ConfiguredRoot returns the root if this instance has it at the configured key and it looks like
// a root (no container). ok is false, with no error, when it cannot be confirmed.
func (c *Client) ConfiguredRoot(ctx context.Context) (key string, ok bool) {
	n, err := c.GetContent(ctx, c.rootKey)
	if err != nil || n.Container != "" {
		return "", false
	}
	return n.Key, n.Key == c.rootKey
}

// maxTreeDepth bounds ancestor walks so a cyclic or corrupt hierarchy cannot loop forever.
const maxTreeDepth = 32

// FindRoot walks container links up from the content item `from` and returns the
// topmost ancestor (the item with no container) and the number of hops taken.
func (c *Client) FindRoot(ctx context.Context, from string) (root string, hops int, err error) {
	key := from
	for ; hops <= maxTreeDepth; hops++ {
		n, err := c.GetContent(ctx, key)
		if err != nil {
			return "", hops, fmt.Errorf("reading content %q while looking for the root: %w", key, err)
		}
		if n.Container == "" {
			return key, hops, nil
		}
		key = n.Container
	}
	return "", hops, fmt.Errorf("content hierarchy above %q is deeper than %d levels; refusing to continue", from, maxTreeDepth)
}

// List returns every item of a paged collection.
func (col *Collection[T]) List(ctx context.Context) ([]T, error) {
	const pageSize = 100
	var all []T
	for page := 0; page < 100; page++ {
		var out struct {
			Items []T `json:"items"`
		}
		p := fmt.Sprintf("%s?pageIndex=%d&pageSize=%d", col.path, page, pageSize)
		if err := col.c.Do(ctx, http.MethodGet, p, "", nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Items...)
		if len(out.Items) < pageSize {
			return all, nil
		}
	}
	return nil, fmt.Errorf("GET %s: more than 100 pages; refusing to continue", col.path)
}

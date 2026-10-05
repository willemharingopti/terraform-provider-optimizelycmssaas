// Package client is a minimal Optimizely CMS (SaaS) REST API client.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultHost is the API host. The client appends the API version itself, so
	// OPTIMIZELY_CMS_BASE_URL is just the host (a trailing /vN is tolerated).
	DefaultHost = "https://api.cms.optimizely.com"
	// DefaultAPIVersion is the API version this provider was built against (spec v1.1).
	DefaultAPIVersion = "v1"
	tokenPath         = "/oauth/token"

	maxResponseBytes = 8 << 20
	maxRetries       = 3
)

// APIError is returned for non-2xx responses other than 404.
type APIError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.Status, truncate(e.Body, 500))
}

// apiVersionRe restricts the version to the form "v1", "v2", ...: it becomes part of every
// request path, so anything else (slashes, dots, query characters) is refused.
var apiVersionRe = regexp.MustCompile(`^v[0-9]{1,3}$`)

// versionSuffixRe matches a trailing version segment on a base URL, e.g. "/v1".
var versionSuffixRe = regexp.MustCompile(`/v[0-9]{1,3}$`)

// ErrNotFound is returned for HTTP 404 responses.
var ErrNotFound = errors.New("not found")

type Client struct {
	rootKey         string
	rootKeyExplicit bool
	apiVersion      string
	baseURL         string
	tokenURL        string
	clientID        string
	clientSecret    string
	http            *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time

	ContentTypes        *Collection[ContentType]
	ContentSources      *Collection[ContentSource]
	ContentTypeBindings *Collection[ContentTypeBinding]
	Locales             *Collection[Locale]
	DisplayTemplates    *Collection[DisplayTemplate]
	Applications        *Collection[Application]
	PropertyGroups      *Collection[PropertyGroup]
	PropertyFormats     *Collection[PropertyFormat]
	Blueprints          *Collection[Blueprint]
}

// Option customises a Client (used by tests).
type Option func(*Client)

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithRootKey overrides the key of the content tree's root (default WellKnownRootKey).
func WithRootKey(k string) Option { return func(c *Client) { c.rootKey = k } }

// WithAPIVersion selects the REST API version, e.g. "v2". It wins over a version suffix on the host.
// The provider is built against v1; other versions may differ and are not tested.
func WithAPIVersion(v string) Option { return func(c *Client) { c.apiVersion = v } }

// New builds a client. host is e.g. https://api.cms.optimizely.com; tokenURL may
// be empty to derive it from host.
func New(host, tokenURL, clientID, clientSecret string, opts ...Option) (*Client, error) {
	if host == "" {
		host = DefaultHost
	}
	host = strings.TrimRight(host, "/")
	hostVersion := strings.TrimPrefix(versionSuffixRe.FindString(host), "/")
	host = versionSuffixRe.ReplaceAllString(host, "")
	if tokenURL == "" {
		tokenURL = host + tokenPath
	}
	for _, u := range []string{host, tokenURL} {
		parsed, err := url.Parse(u)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, fmt.Errorf("URL %q must be a valid https URL", u)
		}
	}
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("client_id and client_secret are required: set them in the provider block or export OPTIMIZELY_CMS_CLIENT_ID and OPTIMIZELY_CMS_CLIENT_SECRET (Terraform does not read .env files)")
	}
	c := &Client{
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	c.rootKeyExplicit = c.rootKey != ""
	if c.rootKey == "" {
		c.rootKey = WellKnownRootKey
	}
	rk, err := normalizeRootKey(c.rootKey)
	if err != nil {
		return nil, err
	}
	c.rootKey = rk
	switch { // an explicit setting wins, then a version on the host, then the default
	case c.apiVersion != "":
	case hostVersion != "":
		c.apiVersion = hostVersion
	default:
		c.apiVersion = DefaultAPIVersion
	}
	if !apiVersionRe.MatchString(c.apiVersion) {
		return nil, fmt.Errorf("api_version %q is invalid: use the form v1, v2, ...", c.apiVersion)
	}
	c.baseURL = host + "/" + c.apiVersion
	c.ContentTypes = &Collection[ContentType]{c: c, path: "/contenttypes"}
	c.ContentSources = &Collection[ContentSource]{c: c, path: "/contentsources"}
	c.ContentTypeBindings = &Collection[ContentTypeBinding]{c: c, path: "/contenttypebindings"}
	c.Locales = &Collection[Locale]{c: c, path: "/locales"}
	c.DisplayTemplates = &Collection[DisplayTemplate]{c: c, path: "/displaytemplates"}
	c.Applications = &Collection[Application]{c: c, path: "/applications"}
	c.PropertyGroups = &Collection[PropertyGroup]{c: c, path: "/propertygroups"}
	c.PropertyFormats = &Collection[PropertyFormat]{c: c, path: "/propertyformats"}
	c.Blueprints = &Collection[Blueprint]{c: c, path: "/blueprints"}
	return c, nil
}

// bearer returns a cached token, refreshing it shortly before expiry.
// Tokens are short-lived (~300s) so we never persist them.
func (c *Client) bearer(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.token != "" && time.Now().Before(c.expiresAt.Add(-30*time.Second)) {
		return c.token, nil
	}

	body, _ := json.Marshal(map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"grant_type":    "client_credentials",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode != http.StatusOK {
		// Deliberately omit the body: it may echo credentials.
		return "", fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil || tr.AccessToken == "" {
		return "", errors.New("token response was invalid")
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 300
	}
	c.token = tr.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.token, nil
}

// Do sends a JSON request to path (relative to the versioned base URL) and
// decodes a non-empty response into out (if non-nil). It retries 429s honouring
// Retry-After, and refreshes the token once on 401. 404 maps to ErrNotFound.
func (c *Client) Do(ctx context.Context, method, path, contentType string, in, out any) error {
	return c.DoH(ctx, method, path, contentType, nil, in, out)
}

// DoH is Do with extra request headers.
func (c *Client) DoH(ctx context.Context, method, path, contentType string, headers map[string]string, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return err
		}
		if contentType == "" {
			contentType = "application/json"
		}
	}

	refreshed := false
	for attempt := 0; ; attempt++ {
		tok, err := c.bearer(ctx, false)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if payload != nil {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusUnauthorized && !refreshed:
			refreshed = true
			if _, err := c.bearer(ctx, true); err != nil {
				return err
			}
			continue
		case resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries:
			wait := time.Duration(1<<attempt) * time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 && s <= 30 {
				wait = time.Duration(s) * time.Second
			}
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		case resp.StatusCode == http.StatusNotFound:
			return ErrNotFound
		case resp.StatusCode >= 300:
			return &APIError{Method: method, Path: path, Status: resp.StatusCode, Body: string(raw)}
		}
		if out != nil && len(raw) > 0 {
			return json.Unmarshal(raw, out)
		}
		return nil
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// Collection is a typed CRUD helper for a keyed REST collection such as /locales.
type Collection[T any] struct {
	c    *Client
	path string
}

var errEmptyKey = errors.New("refusing request with an empty key")

func (col *Collection[T]) item(key string) string { return col.path + "/" + url.PathEscape(key) }

// preferRepresentation asks the API to return the created/changed resource in the
// response body; without it, writes may answer with an empty body.
var preferRepresentation = map[string]string{"Prefer": "return=representation"}

// visibilityWait bounds how long a write waits for reads to catch up. A variable so tests can shorten it.
var visibilityWait = 10 * time.Second

// settledReads is how many consecutive matching reads count as "reads have caught up".
// Reads appear to be served by several replicas that converge at different times, so a
// single good read is not enough.
const settledReads = 3

// modifiedOf extracts lastModified from a JSON representation ("" if absent).
func modifiedOf(raw []byte) string {
	var m struct {
		LastModified string `json:"lastModified"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.LastModified
}

// awaitRead is a read-your-writes barrier. After a write, the API can answer a read with
// 404 or with the previous version for a short while. This polls path until it reads back
// settledReads times in a row with lastModified >= want (any when want is empty). It is
// best effort: it never fails, and gives up after visibilityWait or on an error that
// waiting cannot fix, so a slow API cannot hang an apply.
func (c *Client) awaitRead(ctx context.Context, path, want string) {
	deadline := time.Now().Add(visibilityWait)
	ok := 0
	for time.Now().Before(deadline) {
		var cur struct {
			LastModified string `json:"lastModified"`
		}
		err := c.Do(ctx, http.MethodGet, path, "", nil, &cur)
		switch {
		case err == nil && (want == "" || cur.LastModified >= want):
			if ok++; ok >= settledReads {
				return
			}
		case err == nil || errors.Is(err, ErrNotFound):
			ok = 0 // not there yet, or still the previous version
		default:
			return // a failure waiting will not fix (permissions, outage, ...)
		}
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

// awaitGone is the delete-side barrier: it polls until path reads as 404 settledReads times
// in a row (best effort, bounded by visibilityWait, never fails).
func (c *Client) awaitGone(ctx context.Context, path string) {
	deadline := time.Now().Add(visibilityWait)
	gone := 0
	for time.Now().Before(deadline) {
		err := c.Do(ctx, http.MethodGet, path, "", nil, nil)
		switch {
		case errors.Is(err, ErrNotFound):
			if gone++; gone >= settledReads {
				return
			}
		case err == nil:
			gone = 0 // still readable: the delete has not propagated yet
		default:
			return
		}
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

// Create posts in. key is the new resource's key when known (used to re-read it if
// the API still answers without a body); pass "" when the server generates it.
func (col *Collection[T]) Create(ctx context.Context, key string, in *T) (*T, error) {
	var raw json.RawMessage
	if err := col.c.DoH(ctx, http.MethodPost, col.path, "", preferRepresentation, in, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		if key == "" {
			return nil, fmt.Errorf("POST %s: created, but the API returned no body and no key is known", col.path)
		}
		col.c.awaitRead(ctx, col.item(key), "")
		return col.Get(ctx, key)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	var k struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(raw, &k)
	if k.Key != "" {
		col.c.awaitRead(ctx, col.item(k.Key), modifiedOf(raw))
	}
	return &out, nil
}

func (col *Collection[T]) Get(ctx context.Context, key string) (*T, error) {
	if key == "" {
		return nil, errEmptyKey
	}
	var out T
	if err := col.c.Do(ctx, http.MethodGet, col.item(key), "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Patch sends an RFC 7396 merge-patch (a nil map value deletes that field) and
// returns the resulting resource.
func (col *Collection[T]) Patch(ctx context.Context, key string, patch map[string]any) (*T, error) {
	return col.PatchH(ctx, key, patch, nil)
}

// PatchH is Patch with extra request headers. It asks for the changed resource in
// the response rather than re-reading it: reads straight after a write can return
// the previous version. Only when the API still answers without a body does it
// fall back to a GET.
func (col *Collection[T]) PatchH(ctx context.Context, key string, patch map[string]any, headers map[string]string) (*T, error) {
	if key == "" {
		return nil, errEmptyKey
	}
	h := map[string]string{"Prefer": preferRepresentation["Prefer"]}
	for k, v := range headers {
		h[k] = v
	}
	var raw json.RawMessage
	if err := col.c.DoH(ctx, http.MethodPatch, col.item(key), "application/merge-patch+json", h, patch, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		col.c.awaitRead(ctx, col.item(key), "")
		return col.Get(ctx, key)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	col.c.awaitRead(ctx, col.item(key), modifiedOf(raw))
	return &out, nil
}

func (col *Collection[T]) Delete(ctx context.Context, key string) error {
	if key == "" {
		return errEmptyKey
	}
	if err := col.c.Do(ctx, http.MethodDelete, col.item(key), "", nil, nil); err != nil {
		return err
	}
	col.c.awaitGone(ctx, col.item(key))
	return nil
}

// ---- API models (see openapi.json, v1.1) ----

type SecurityIdentity struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type ContentType struct {
	Key                  string                     `json:"key"`
	DisplayName          string                     `json:"displayName"`
	Description          string                     `json:"description,omitempty"`
	BaseType             string                     `json:"baseType,omitempty"`
	IsContract           bool                       `json:"isContract,omitempty"`
	SortOrder            *int32                     `json:"sortOrder,omitempty"`
	MayContainTypes      []string                   `json:"mayContainTypes,omitempty"`
	MediaFileExtensions  []string                   `json:"mediaFileExtensions,omitempty"`
	CompositionBehaviors []string                   `json:"compositionBehaviors,omitempty"`
	Contracts            []string                   `json:"contracts,omitempty"`
	Properties           map[string]json.RawMessage `json:"properties,omitempty"`
	AccessRights         []SecurityIdentity         `json:"accessRights,omitempty"`
}

type PropertyMappings struct {
	Key         string `json:"key,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	KeyFormat   string `json:"keyFormat,omitempty"`
}

type ContentSource struct {
	Key              string           `json:"key"`
	Type             string           `json:"type"`
	SourceKey        string           `json:"sourceKey"`
	SourceType       string           `json:"sourceType"`
	DisplayName      string           `json:"displayName"`
	BaseType         string           `json:"baseType"`
	PropertyMappings PropertyMappings `json:"propertyMappings"`
}

type PropertyMapping struct {
	From    string `json:"from"`
	Binding string `json:"binding,omitempty"`
}

type ContentTypeBinding struct {
	Key              string                     `json:"key"`
	From             string                     `json:"from"`
	To               string                     `json:"to"`
	PropertyMappings map[string]PropertyMapping `json:"propertyMappings,omitempty"`
}

type Locale struct {
	Key          string             `json:"key"`
	DisplayName  string             `json:"displayName"`
	RouteSegment string             `json:"routeSegment"`
	IsEnabled    *bool              `json:"isEnabled,omitempty"`
	SortOrder    *int32             `json:"sortOrder,omitempty"`
	Fallback     *string            `json:"fallback,omitempty"`
	AccessRights []SecurityIdentity `json:"accessRights,omitempty"`
}

type DisplaySettingChoice struct {
	DisplayName string `json:"displayName"`
	SortOrder   int32  `json:"sortOrder"`
}

type DisplaySetting struct {
	DisplayName string                          `json:"displayName"`
	Editor      string                          `json:"editor,omitempty"`
	SortOrder   int32                           `json:"sortOrder"`
	Choices     map[string]DisplaySettingChoice `json:"choices,omitempty"`
}

type DisplayTemplate struct {
	Key         string                    `json:"key"`
	DisplayName string                    `json:"displayName"`
	NodeType    string                    `json:"nodeType,omitempty"`
	BaseType    string                    `json:"baseType,omitempty"`
	ContentType string                    `json:"contentType,omitempty"`
	IsDefault   *bool                     `json:"isDefault,omitempty"`
	Settings    map[string]DisplaySetting `json:"settings,omitempty"`
}

type ApplicationHost struct {
	Authority          string `json:"authority"`
	Type               string `json:"type,omitempty"`
	Locale             string `json:"locale,omitempty"`
	PreferredURLScheme string `json:"preferredUrlScheme,omitempty"`
}

type Application struct {
	Key                          string            `json:"key"`
	DisplayName                  string            `json:"displayName"`
	Type                         string            `json:"type"`
	EntryPoint                   string            `json:"entryPoint"`
	IsDefault                    bool              `json:"isDefault"`
	UseApplicationSpecificAssets bool              `json:"useApplicationSpecificAssets"`
	AssetsRoot                   string            `json:"assetsRoot,omitempty"` // read-only
	Hosts                        []ApplicationHost `json:"hosts,omitempty"`
	UsePreviewTokens             *bool             `json:"usePreviewTokens,omitempty"`
	PreviewURLFormats            map[string]string `json:"previewUrlFormats,omitempty"`
}

type PropertyGroup struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	SortOrder   *int32 `json:"sortOrder,omitempty"`
}

// PropertyFormat is read-only in the API.
type PropertyFormat struct {
	Key         string `json:"key"`
	DataType    string `json:"dataType,omitempty"`
	ItemType    string `json:"itemType,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	IsDeleted   bool   `json:"isDeleted,omitempty"`
}

// Blueprint.Content is the BlueprintData object (properties, binding, composition).
type Blueprint struct {
	Key         string          `json:"key,omitempty"`
	DisplayName string          `json:"displayName"`
	ContentType string          `json:"contentType"`
	Content     json.RawMessage `json:"content"`
}

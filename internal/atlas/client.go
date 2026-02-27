package atlas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://cloud.mongodb.com"
	defaultTimeout = 30 * time.Second
)

// Client wraps Atlas Admin API v2 calls used by audit enrichment.
type Client struct {
	baseURL     *url.URL
	httpClient  *http.Client
	minInterval time.Duration

	mu          sync.Mutex
	lastRequest time.Time
}

// NewClient constructs an Atlas API client with digest auth.
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.PublicKey) == "" || strings.TrimSpace(cfg.PrivateKey) == "" {
		return nil, fmt.Errorf("atlas credentials are required")
	}

	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = defaultBaseURL
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid atlas base url: %w", err)
	}

	intervalMS := cfg.RateLimitMS
	if intervalMS <= 0 {
		intervalMS = 250
	}

	transport := newDigestTransport(cfg.PublicKey, cfg.PrivateKey, http.DefaultTransport)
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   defaultTimeout,
	}

	return &Client{
		baseURL:     baseURL,
		httpClient:  httpClient,
		minInterval: time.Duration(intervalMS) * time.Millisecond,
	}, nil
}

// ListProjects returns Atlas projects (groups) visible to the API key.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var envelope listEnvelope[map[string]any]
	if err := c.get(ctx, "/api/atlas/v2/groups", url.Values{
		"itemsPerPage": []string{"500"},
		"includeCount": []string{"false"},
		"pretty":       []string{"false"},
	}, &envelope); err != nil {
		return nil, err
	}

	projects := make([]Project, 0, len(envelope.Results))
	for _, raw := range envelope.Results {
		project := Project{
			ID:   firstString(raw, "id", "groupId"),
			Name: firstString(raw, "name"),
		}
		if project.ID == "" {
			continue
		}
		projects = append(projects, project)
	}
	return projects, nil
}

// ListClusters lists clusters in a project.
func (c *Client) ListClusters(ctx context.Context, projectID string) ([]Cluster, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("atlas project id is required")
	}

	path := fmt.Sprintf("/api/atlas/v2/groups/%s/clusters", url.PathEscape(projectID))
	var envelope listEnvelope[map[string]any]
	if err := c.get(ctx, path, url.Values{
		"itemsPerPage": []string{"200"},
		"includeCount": []string{"false"},
	}, &envelope); err != nil {
		return nil, err
	}

	clusters := make([]Cluster, 0, len(envelope.Results))
	for _, raw := range envelope.Results {
		cluster := parseCluster(raw)
		if cluster.Name == "" {
			continue
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}

// GetCluster returns one Atlas cluster configuration.
func (c *Client) GetCluster(ctx context.Context, projectID, clusterName string) (Cluster, error) {
	if strings.TrimSpace(projectID) == "" {
		return Cluster{}, fmt.Errorf("atlas project id is required")
	}
	if strings.TrimSpace(clusterName) == "" {
		return Cluster{}, fmt.Errorf("atlas cluster name is required")
	}

	path := fmt.Sprintf("/api/atlas/v2/groups/%s/clusters/%s", url.PathEscape(projectID), url.PathEscape(clusterName))
	var raw map[string]any
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return Cluster{}, err
	}
	cluster := parseCluster(raw)
	if cluster.Name == "" {
		cluster.Name = clusterName
	}
	return cluster, nil
}

// ListSuggestedIndexes returns Atlas Performance Advisor index recommendations.
func (c *Client) ListSuggestedIndexes(ctx context.Context, projectID, clusterName string) ([]SuggestedIndex, error) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(clusterName) == "" {
		return nil, fmt.Errorf("atlas project and cluster are required")
	}

	path := fmt.Sprintf("/api/atlas/v2/groups/%s/clusters/%s/performanceAdvisor/suggestedIndexes", url.PathEscape(projectID), url.PathEscape(clusterName))
	var envelope listEnvelope[map[string]any]
	if err := c.get(ctx, path, url.Values{
		"itemsPerPage": []string{"200"},
		"includeCount": []string{"false"},
	}, &envelope); err != nil {
		return nil, err
	}

	items := make([]SuggestedIndex, 0, len(envelope.Results))
	for _, raw := range envelope.Results {
		si := SuggestedIndex{
			Namespace: firstString(raw, "namespace", "collectionNamespace", "ns"),
		}
		if si.Namespace == "" {
			dbName := firstString(raw, "db", "database", "dbName")
			collName := firstString(raw, "collection", "collectionName")
			if dbName != "" && collName != "" {
				si.Namespace = dbName + "." + collName
			} else if collName != "" {
				si.Namespace = collName
			}
		}
		si.IndexFields = append(si.IndexFields, parseIndexFields(raw["index"])...)
		if len(si.IndexFields) == 0 {
			si.IndexFields = append(si.IndexFields, parseIndexFields(raw["keys"])...)
		}
		if len(si.IndexFields) == 0 {
			si.IndexFields = append(si.IndexFields, parseIndexFields(raw["suggestedIndex"])...)
		}
		if si.Namespace == "" || len(si.IndexFields) == 0 {
			continue
		}
		items = append(items, si)
	}
	return items, nil
}

// ListAlerts returns Atlas alerts for a project.
func (c *Client) ListAlerts(ctx context.Context, projectID string) ([]Alert, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("atlas project id is required")
	}

	path := fmt.Sprintf("/api/atlas/v2/groups/%s/alerts", url.PathEscape(projectID))
	var envelope listEnvelope[map[string]any]
	if err := c.get(ctx, path, url.Values{
		"itemsPerPage": []string{"200"},
		"includeCount": []string{"false"},
	}, &envelope); err != nil {
		return nil, err
	}

	alerts := make([]Alert, 0, len(envelope.Results))
	for _, raw := range envelope.Results {
		alert := Alert{
			ID:            firstString(raw, "id", "_id"),
			EventTypeName: firstString(raw, "eventTypeName", "eventType"),
			Status:        firstString(raw, "status"),
		}
		if alert.EventTypeName == "" {
			continue
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

// ListMongoDBVersions returns project MongoDB versions available in Atlas.
func (c *Client) ListMongoDBVersions(ctx context.Context, projectID string) ([]string, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("atlas project id is required")
	}

	path := fmt.Sprintf("/api/atlas/v2/groups/%s/mongoDBVersions", url.PathEscape(projectID))
	var envelope listEnvelope[any]
	if err := c.get(ctx, path, url.Values{
		"itemsPerPage": []string{"100"},
		"includeCount": []string{"false"},
	}, &envelope); err != nil {
		return nil, err
	}

	versions := make([]string, 0, len(envelope.Results))
	for _, raw := range envelope.Results {
		switch v := raw.(type) {
		case string:
			if v != "" {
				versions = append(versions, v)
			}
		case map[string]any:
			version := firstString(v, "version", "name", "mongoDBVersion")
			if version != "" {
				versions = append(versions, version)
			}
		}
	}
	return versions, nil
}

// ListDatabaseUsers returns all database users in an Atlas project.
func (c *Client) ListDatabaseUsers(ctx context.Context, projectID string) ([]DatabaseUser, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("atlas project id is required")
	}

	path := fmt.Sprintf("/api/atlas/v2/groups/%s/databaseUsers", url.PathEscape(projectID))
	var envelope listEnvelope[map[string]any]
	if err := c.get(ctx, path, url.Values{
		"itemsPerPage": []string{"500"},
		"includeCount": []string{"false"},
	}, &envelope); err != nil {
		return nil, err
	}

	users := make([]DatabaseUser, 0, len(envelope.Results))
	for _, raw := range envelope.Results {
		user := DatabaseUser{
			Username:        firstString(raw, "username"),
			DatabaseName:    firstString(raw, "databaseName"),
			GroupID:         firstString(raw, "groupId"),
			DeleteAfterDate: firstString(raw, "deleteAfterDate"),
		}
		if user.Username == "" {
			continue
		}
		for _, r := range toSlice(raw["roles"]) {
			rm := toMap(r)
			if rm == nil {
				continue
			}
			user.Roles = append(user.Roles, DatabaseUserRole{
				RoleName:     firstString(rm, "roleName"),
				DatabaseName: firstString(rm, "databaseName"),
			})
		}
		for _, s := range toSlice(raw["scopes"]) {
			sm := toMap(s)
			if sm == nil {
				continue
			}
			user.Scopes = append(user.Scopes, DatabaseUserScope{
				Name: firstString(sm, "name"),
				Type: firstString(sm, "type"),
			})
		}
		users = append(users, user)
	}
	return users, nil
}

// ListAccessLogs returns database authentication events for an Atlas cluster.
// Atlas retains access logs for 7 days and returns max 25 entries per page.
func (c *Client) ListAccessLogs(ctx context.Context, projectID, clusterName string) ([]AccessLogEntry, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("atlas project id is required")
	}
	if strings.TrimSpace(clusterName) == "" {
		return nil, fmt.Errorf("atlas cluster name is required")
	}

	path := fmt.Sprintf("/api/atlas/v2/groups/%s/dbAccessHistory/clusters/%s",
		url.PathEscape(projectID), url.PathEscape(clusterName))

	const pageSize = 25
	var allEntries []AccessLogEntry

	for pageNum := 1; ; pageNum++ {
		var envelope listEnvelope[map[string]any]
		if err := c.get(ctx, path, url.Values{
			"itemsPerPage": []string{strconv.Itoa(pageSize)},
			"pageNum":      []string{strconv.Itoa(pageNum)},
			"includeCount": []string{"false"},
		}, &envelope); err != nil {
			return nil, err
		}

		for _, raw := range envelope.Results {
			entry := AccessLogEntry{
				AuthSource:    firstString(raw, "authSource"),
				Username:      firstString(raw, "username"),
				Timestamp:     firstString(raw, "timestamp"),
				IPAddress:     firstString(raw, "ipAddress"),
				FailureReason: firstString(raw, "failureReason"),
			}
			if v, ok := raw["authResult"].(bool); ok {
				entry.AuthResult = v
			}
			if entry.Username == "" {
				continue
			}
			allEntries = append(allEntries, entry)
		}

		if len(envelope.Results) < pageSize {
			break
		}
	}

	return allEntries, nil
}

// ResolveProjectIDByCluster discovers a project containing the given cluster.
func (c *Client) ResolveProjectIDByCluster(ctx context.Context, clusterName string) (string, error) {
	clusterName = strings.TrimSpace(clusterName)
	if clusterName == "" {
		return "", fmt.Errorf("atlas cluster name is required")
	}

	projects, err := c.ListProjects(ctx)
	if err != nil {
		return "", err
	}

	for _, project := range projects {
		_, err := c.GetCluster(ctx, project.ID, clusterName)
		if err == nil {
			return project.ID, nil
		}
		if IsStatus(err, http.StatusNotFound) {
			continue
		}
	}

	return "", fmt.Errorf("atlas project for cluster %q not found", clusterName)
}

type listEnvelope[T any] struct {
	Results []T `json:"results"`
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	if err := c.waitRateLimit(ctx); err != nil {
		return err
	}

	u := *c.baseURL
	u.Path = strings.TrimRight(c.baseURL.Path, "/") + path
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.atlas.2024-10-23+json")
	req.Header.Set("User-Agent", "mongospectre")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		apiErr := &APIError{StatusCode: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if len(body) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err == nil {
				apiErr.Code = firstString(payload, "errorCode", "code")
				apiErr.Message = firstString(payload, "detail", "error", "reason", "message")
			}
		}
		if apiErr.Message == "" {
			apiErr.Message = http.StatusText(resp.StatusCode)
		}
		return apiErr
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode atlas response: %w", err)
	}
	return nil
}

func (c *Client) waitRateLimit(ctx context.Context) error {
	if c.minInterval <= 0 {
		return nil
	}

	for {
		c.mu.Lock()
		wait := c.minInterval - time.Since(c.lastRequest)
		if wait <= 0 {
			c.lastRequest = time.Now()
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func parseCluster(raw map[string]any) Cluster {
	cluster := Cluster{
		ID:             firstString(raw, "id"),
		Name:           firstString(raw, "name"),
		MongoDBVersion: firstString(raw, "mongoDBVersion"),
	}

	cluster.InstanceSizeName = firstInstanceSize(raw)
	return cluster
}

func firstInstanceSize(raw map[string]any) string {
	if provider := toMap(raw["providerSettings"]); provider != nil {
		if size := firstString(provider, "instanceSizeName", "instanceSize"); size != "" {
			return size
		}
	}

	for _, rep := range toSlice(raw["replicationSpecs"]) {
		repMap := toMap(rep)
		for _, region := range toSlice(repMap["regionConfigs"]) {
			regionMap := toMap(region)
			for _, key := range []string{"electableSpecs", "effectiveElectableSpecs", "analyticsSpecs", "readOnlySpecs"} {
				spec := toMap(regionMap[key])
				if size := firstString(spec, "instanceSize", "instanceSizeName"); size != "" {
					return size
				}
			}
		}
	}

	return ""
}

func parseIndexFields(v any) []string {
	if v == nil {
		return nil
	}

	var fields []string
	seen := map[string]bool{}

	add := func(field string) {
		field = strings.TrimSpace(field)
		if field == "" {
			return
		}
		if !seen[field] {
			seen[field] = true
			fields = append(fields, field)
		}
	}

	switch x := v.(type) {
	case []any:
		for _, entry := range x {
			switch idx := entry.(type) {
			case string:
				add(idx)
			case map[string]any:
				keys := make([]string, 0, len(idx))
				for k := range idx {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if strings.EqualFold(k, "field") {
						if fv := toString(idx[k]); fv != "" {
							add(fv)
							continue
						}
					}
					add(k)
				}
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add(k)
		}
	}

	return fields
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := toString(m[k]); v != "" {
			return v
		}
	}
	return ""
}

func toSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func toMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case float64:
		if math.Trunc(x) == x {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	default:
		return ""
	}
}

package atlas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(Config{
		PublicKey:  "testpublic",
		PrivateKey: "testprivate",
		BaseURL:    server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestListDatabaseUsers(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/atlas/v2/groups/proj123/databaseUsers" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"results": []map[string]any{
				{
					"username":     "admin",
					"databaseName": "admin",
					"groupId":      "proj123",
					"roles": []map[string]any{
						{"roleName": "readWriteAnyDatabase", "databaseName": "admin"},
						{"roleName": "dbAdminAnyDatabase", "databaseName": "admin"},
					},
					"scopes": []map[string]any{
						{"name": "Cluster0", "type": "CLUSTER"},
					},
				},
				{
					"username":     "appuser",
					"databaseName": "admin",
					"groupId":      "proj123",
					"roles": []map[string]any{
						{"roleName": "readWrite", "databaseName": "myapp"},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	client := newTestClient(t, handler)
	users, err := client.ListDatabaseUsers(context.Background(), "proj123")
	if err != nil {
		t.Fatalf("ListDatabaseUsers: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	if users[0].Username != "admin" {
		t.Errorf("expected username 'admin', got %q", users[0].Username)
	}
	if users[0].DatabaseName != "admin" {
		t.Errorf("expected databaseName 'admin', got %q", users[0].DatabaseName)
	}
	if len(users[0].Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(users[0].Roles))
	}
	if users[0].Roles[0].RoleName != "readWriteAnyDatabase" {
		t.Errorf("expected role 'readWriteAnyDatabase', got %q", users[0].Roles[0].RoleName)
	}
	if len(users[0].Scopes) != 1 {
		t.Errorf("expected 1 scope, got %d", len(users[0].Scopes))
	}
	if users[0].Scopes[0].Name != "Cluster0" {
		t.Errorf("expected scope name 'Cluster0', got %q", users[0].Scopes[0].Name)
	}

	if users[1].Username != "appuser" {
		t.Errorf("expected username 'appuser', got %q", users[1].Username)
	}
	if len(users[1].Scopes) != 0 {
		t.Errorf("expected 0 scopes for appuser, got %d", len(users[1].Scopes))
	}
}

func TestListDatabaseUsers_Empty(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{"results": []any{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	client := newTestClient(t, handler)
	users, err := client.ListDatabaseUsers(context.Background(), "proj123")
	if err != nil {
		t.Fatalf("ListDatabaseUsers: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestListDatabaseUsers_APIError(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		resp := map[string]any{
			"errorCode": "FORBIDDEN",
			"detail":    "IP address not on access list",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	client := newTestClient(t, handler)
	_, err := client.ListDatabaseUsers(context.Background(), "proj123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403 status, got: %v", err)
	}
}

func TestListDatabaseUsers_EmptyProjectID(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	_, err := client.ListDatabaseUsers(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty project ID")
	}
}

func TestListDatabaseUsers_SkipsEmptyUsername(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"results": []map[string]any{
				{"username": "", "databaseName": "admin"},
				{"username": "validuser", "databaseName": "admin"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	client := newTestClient(t, handler)
	users, err := client.ListDatabaseUsers(context.Background(), "proj123")
	if err != nil {
		t.Fatalf("ListDatabaseUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user (empty username skipped), got %d", len(users))
	}
	if users[0].Username != "validuser" {
		t.Errorf("expected 'validuser', got %q", users[0].Username)
	}
}

func TestListAccessLogs(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/atlas/v2/groups/proj123/dbAccessHistory/clusters/Cluster0" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"results": []map[string]any{
				{
					"authResult":    true,
					"authSource":    "admin",
					"username":      "appuser",
					"timestamp":     "2026-02-27T10:00:00Z",
					"ipAddress":     "10.0.0.1",
					"failureReason": "",
				},
				{
					"authResult":    false,
					"authSource":    "admin",
					"username":      "baduser",
					"timestamp":     "2026-02-27T09:00:00Z",
					"ipAddress":     "10.0.0.2",
					"failureReason": "AuthenticationFailed",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	client := newTestClient(t, handler)
	logs, err := client.ListAccessLogs(context.Background(), "proj123", "Cluster0")
	if err != nil {
		t.Fatalf("ListAccessLogs: %v", err)
	}

	if len(logs) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(logs))
	}
	if !logs[0].AuthResult {
		t.Errorf("expected authResult true for appuser")
	}
	if logs[0].Username != "appuser" {
		t.Errorf("expected username 'appuser', got %q", logs[0].Username)
	}
	if logs[1].AuthResult {
		t.Errorf("expected authResult false for baduser")
	}
	if logs[1].FailureReason != "AuthenticationFailed" {
		t.Errorf("expected failureReason 'AuthenticationFailed', got %q", logs[1].FailureReason)
	}
}

func TestListAccessLogs_Pagination(t *testing.T) {
	callCount := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		callCount++
		pageNum := r.URL.Query().Get("pageNum")

		var results []map[string]any
		if pageNum == "1" {
			// Return full page (25 entries) to trigger next page fetch.
			for i := range 25 {
				results = append(results, map[string]any{
					"authResult": true,
					"authSource": "admin",
					"username":   "user" + string(rune('A'+i)),
					"timestamp":  "2026-02-27T10:00:00Z",
					"ipAddress":  "10.0.0.1",
				})
			}
		} else {
			// Second page with fewer than 25 — signals end.
			results = []map[string]any{
				{
					"authResult": true,
					"authSource": "admin",
					"username":   "lastuser",
					"timestamp":  "2026-02-20T10:00:00Z",
					"ipAddress":  "10.0.0.2",
				},
			}
		}

		resp := map[string]any{"results": results}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	client := newTestClient(t, handler)
	logs, err := client.ListAccessLogs(context.Background(), "proj123", "Cluster0")
	if err != nil {
		t.Fatalf("ListAccessLogs: %v", err)
	}

	if len(logs) != 26 {
		t.Fatalf("expected 26 entries (25 + 1), got %d", len(logs))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (2 pages), got %d", callCount)
	}
}

func TestListAccessLogs_Empty(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{"results": []any{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	client := newTestClient(t, handler)
	logs, err := client.ListAccessLogs(context.Background(), "proj123", "Cluster0")
	if err != nil {
		t.Fatalf("ListAccessLogs: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 entries, got %d", len(logs))
	}
}

func TestListAccessLogs_APIError(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		resp := map[string]any{
			"errorCode": "FORBIDDEN",
			"detail":    "Insufficient permissions",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	client := newTestClient(t, handler)
	_, err := client.ListAccessLogs(context.Background(), "proj123", "Cluster0")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403 status, got: %v", err)
	}
}

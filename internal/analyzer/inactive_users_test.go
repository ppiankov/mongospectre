package analyzer

import (
	"testing"

	"github.com/ppiankov/mongospectre/internal/atlas"
)

func TestDetectInactiveUsers_AllActive(t *testing.T) {
	users := []atlas.DatabaseUser{
		{Username: "app", DatabaseName: "admin", Roles: []atlas.DatabaseUserRole{{RoleName: "readWrite", DatabaseName: "myapp"}}},
		{Username: "svc", DatabaseName: "admin", Roles: []atlas.DatabaseUserRole{{RoleName: "read", DatabaseName: "myapp"}}},
	}
	logs := []atlas.AccessLogEntry{
		{Username: "app", AuthResult: true, Timestamp: "2026-02-27T10:00:00Z"},
		{Username: "svc", AuthResult: true, Timestamp: "2026-02-27T09:00:00Z"},
	}

	findings := DetectInactiveUsers(users, logs)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for all active users, got %d", len(findings))
	}
}

func TestDetectInactiveUsers_Inactive(t *testing.T) {
	users := []atlas.DatabaseUser{
		{Username: "active", DatabaseName: "admin", Roles: []atlas.DatabaseUserRole{{RoleName: "readWrite", DatabaseName: "myapp"}}},
		{Username: "stale", DatabaseName: "admin", Roles: []atlas.DatabaseUserRole{{RoleName: "read", DatabaseName: "reports"}}},
	}
	logs := []atlas.AccessLogEntry{
		{Username: "active", AuthResult: true, Timestamp: "2026-02-27T10:00:00Z"},
	}

	findings := DetectInactiveUsers(users, logs)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingInactiveUser {
		t.Errorf("expected INACTIVE_USER, got %s", findings[0].Type)
	}
	if findings[0].Severity != SeverityMedium {
		t.Errorf("expected medium severity, got %s", findings[0].Severity)
	}
}

func TestDetectInactiveUsers_InactivePrivileged(t *testing.T) {
	users := []atlas.DatabaseUser{
		{
			Username:     "old_admin",
			DatabaseName: "admin",
			Roles:        []atlas.DatabaseUserRole{{RoleName: "root", DatabaseName: "admin"}},
		},
	}

	findings := DetectInactiveUsers(users, nil)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingInactivePrivilegedUser {
		t.Errorf("expected INACTIVE_PRIVILEGED_USER, got %s", findings[0].Type)
	}
	if findings[0].Severity != SeverityHigh {
		t.Errorf("expected high severity, got %s", findings[0].Severity)
	}
}

func TestDetectInactiveUsers_FailedAuthOnly(t *testing.T) {
	users := []atlas.DatabaseUser{
		{Username: "broken", DatabaseName: "admin", Roles: []atlas.DatabaseUserRole{{RoleName: "readWrite", DatabaseName: "myapp"}}},
	}
	logs := []atlas.AccessLogEntry{
		{Username: "broken", AuthResult: false, FailureReason: "AuthenticationFailed", Timestamp: "2026-02-27T10:00:00Z"},
		{Username: "broken", AuthResult: false, FailureReason: "AuthenticationFailed", Timestamp: "2026-02-27T08:00:00Z"},
	}

	findings := DetectInactiveUsers(users, logs)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingFailedAuthOnly {
		t.Errorf("expected FAILED_AUTH_ONLY, got %s", findings[0].Type)
	}
}

func TestDetectInactiveUsers_EmptyLogs(t *testing.T) {
	users := []atlas.DatabaseUser{
		{Username: "admin", DatabaseName: "admin", Roles: []atlas.DatabaseUserRole{{RoleName: "root", DatabaseName: "admin"}}},
		{Username: "app", DatabaseName: "admin", Roles: []atlas.DatabaseUserRole{{RoleName: "readWrite", DatabaseName: "myapp"}}},
	}

	findings := DetectInactiveUsers(users, nil)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	// admin with root → INACTIVE_PRIVILEGED_USER (high)
	if findings[0].Type != FindingInactivePrivilegedUser {
		t.Errorf("expected INACTIVE_PRIVILEGED_USER for root user, got %s", findings[0].Type)
	}
	// app with readWrite → INACTIVE_USER (medium)
	if findings[1].Type != FindingInactiveUser {
		t.Errorf("expected INACTIVE_USER for app user, got %s", findings[1].Type)
	}
}

func TestDetectInactiveUsers_EmptyUsers(t *testing.T) {
	logs := []atlas.AccessLogEntry{
		{Username: "someone", AuthResult: true, Timestamp: "2026-02-27T10:00:00Z"},
	}

	findings := DetectInactiveUsers(nil, logs)
	if findings != nil {
		t.Errorf("expected nil findings for empty users, got %d", len(findings))
	}
}

func TestDetectInactiveUsers_ReadWriteAnyDatabase(t *testing.T) {
	users := []atlas.DatabaseUser{
		{
			Username:     "wide_user",
			DatabaseName: "admin",
			Roles:        []atlas.DatabaseUserRole{{RoleName: "readWriteAnyDatabase", DatabaseName: "admin"}},
		},
	}

	findings := DetectInactiveUsers(users, nil)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingInactivePrivilegedUser {
		t.Errorf("expected INACTIVE_PRIVILEGED_USER for readWriteAnyDatabase user, got %s", findings[0].Type)
	}
}

package analyzer

import (
	"testing"

	"github.com/ppiankov/mongospectre/internal/atlas"
)

func TestAuditAtlasUsers_NoScope(t *testing.T) {
	users := []atlas.DatabaseUser{
		{
			Username:     "unscoped",
			DatabaseName: "admin",
			Roles: []atlas.DatabaseUserRole{
				{RoleName: "readWrite", DatabaseName: "myapp"},
			},
			// No Scopes — unrestricted access to all clusters.
		},
	}

	findings := AuditAtlasUsers(users)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingAtlasUserNoScope {
		t.Errorf("expected ATLAS_USER_NO_SCOPE, got %s", findings[0].Type)
	}
	if findings[0].Severity != SeverityInfo {
		t.Errorf("expected info severity, got %s", findings[0].Severity)
	}
}

func TestAuditAtlasUsers_Scoped(t *testing.T) {
	users := []atlas.DatabaseUser{
		{
			Username:     "scoped",
			DatabaseName: "admin",
			Roles: []atlas.DatabaseUserRole{
				{RoleName: "readWrite", DatabaseName: "myapp"},
			},
			Scopes: []atlas.DatabaseUserScope{
				{Name: "Cluster0", Type: "CLUSTER"},
			},
		},
	}

	findings := AuditAtlasUsers(users)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for scoped user, got %d: %+v", len(findings), findings)
	}
}

func TestAuditAtlasUsers_NoRoles(t *testing.T) {
	// User with no roles and no scopes should NOT trigger (no roles = harmless).
	users := []atlas.DatabaseUser{
		{
			Username:     "noroles",
			DatabaseName: "admin",
		},
	}

	findings := AuditAtlasUsers(users)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for user with no roles, got %d", len(findings))
	}
}

func TestAuditAtlasUsers_Empty(t *testing.T) {
	findings := AuditAtlasUsers(nil)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for nil users, got %d", len(findings))
	}
}

func TestAuditAtlasUsers_MultipleUsers(t *testing.T) {
	users := []atlas.DatabaseUser{
		{
			Username:     "scoped",
			DatabaseName: "admin",
			Roles:        []atlas.DatabaseUserRole{{RoleName: "readWrite", DatabaseName: "myapp"}},
			Scopes:       []atlas.DatabaseUserScope{{Name: "Cluster0", Type: "CLUSTER"}},
		},
		{
			Username:     "unscoped1",
			DatabaseName: "admin",
			Roles:        []atlas.DatabaseUserRole{{RoleName: "root", DatabaseName: "admin"}},
		},
		{
			Username:     "unscoped2",
			DatabaseName: "admin",
			Roles:        []atlas.DatabaseUserRole{{RoleName: "dbAdmin", DatabaseName: "myapp"}},
		},
	}

	findings := AuditAtlasUsers(users)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings for 2 unscoped users, got %d", len(findings))
	}
	for _, f := range findings {
		if f.Type != FindingAtlasUserNoScope {
			t.Errorf("expected ATLAS_USER_NO_SCOPE, got %s", f.Type)
		}
	}
}

package analyzer

import (
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func TestAuditUsers_AdminInDataDB(t *testing.T) {
	users := []mongoinspect.UserInfo{
		{Username: "appAdmin", Database: "myapp", Roles: []mongoinspect.UserRole{
			{Role: "dbOwner", DB: "myapp"},
		}},
		{Username: "normalUser", Database: "myapp", Roles: []mongoinspect.UserRole{
			{Role: "readWrite", DB: "myapp"},
		}},
		{Username: "adminUser", Database: "admin", Roles: []mongoinspect.UserRole{
			{Role: "root", DB: "admin"},
		}},
	}

	findings := AuditUsers(users)

	var found bool
	for _, f := range findings {
		if f.Type == FindingAdminInDataDB && f.Database == "myapp" {
			found = true
			if f.Severity != SeverityHigh {
				t.Errorf("expected high severity, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected ADMIN_IN_DATA_DB finding for appAdmin in myapp")
	}

	// normalUser should not trigger ADMIN_IN_DATA_DB
	for _, f := range findings {
		if f.Type == FindingAdminInDataDB && f.Message == `user "normalUser" has admin role "readWrite" in non-admin database` {
			t.Error("readWrite should not trigger ADMIN_IN_DATA_DB")
		}
	}

	// adminUser in admin db should not trigger ADMIN_IN_DATA_DB
	for _, f := range findings {
		if f.Type == FindingAdminInDataDB && f.Database == "admin" {
			t.Error("users in admin db should not trigger ADMIN_IN_DATA_DB")
		}
	}
}

func TestAuditUsers_DuplicateUser(t *testing.T) {
	users := []mongoinspect.UserInfo{
		{Username: "shared", Database: "admin", Roles: []mongoinspect.UserRole{
			{Role: "readWrite", DB: "myapp"},
		}},
		{Username: "shared", Database: "myapp", Roles: []mongoinspect.UserRole{
			{Role: "readWrite", DB: "myapp"},
		}},
		{Username: "unique", Database: "admin", Roles: []mongoinspect.UserRole{
			{Role: "readWrite", DB: "myapp"},
		}},
	}

	findings := AuditUsers(users)

	var found bool
	for _, f := range findings {
		if f.Type == FindingDuplicateUser {
			found = true
			if f.Severity != SeverityMedium {
				t.Errorf("expected medium severity, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected DUPLICATE_USER finding for 'shared'")
	}
}

func TestAuditUsers_OverprivilegedUser(t *testing.T) {
	users := []mongoinspect.UserInfo{
		{Username: "superAdmin", Database: "admin", Roles: []mongoinspect.UserRole{
			{Role: "root", DB: "admin"},
		}},
		{Username: "appUser", Database: "myapp", Roles: []mongoinspect.UserRole{
			{Role: "readWrite", DB: "myapp"},
		}},
	}

	findings := AuditUsers(users)

	var found bool
	for _, f := range findings {
		if f.Type == FindingOverprivilegedUser {
			found = true
			if f.Severity != SeverityMedium {
				t.Errorf("expected medium severity, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected OVERPRIVILEGED_USER finding for superAdmin")
	}

	// appUser with readWrite should not trigger
	for _, f := range findings {
		if f.Type == FindingOverprivilegedUser && f.Database == "myapp" {
			t.Error("readWrite should not trigger OVERPRIVILEGED_USER")
		}
	}
}

func TestAuditUsers_MultipleAdminUsers(t *testing.T) {
	users := []mongoinspect.UserInfo{
		{Username: "admin1", Database: "admin", Roles: []mongoinspect.UserRole{
			{Role: "root", DB: "admin"},
		}},
		{Username: "admin2", Database: "admin", Roles: []mongoinspect.UserRole{
			{Role: "clusterAdmin", DB: "admin"},
		}},
	}

	findings := AuditUsers(users)

	var found bool
	for _, f := range findings {
		if f.Type == FindingMultipleAdminUsers {
			found = true
			if f.Severity != SeverityMedium {
				t.Errorf("expected medium severity, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected MULTIPLE_ADMIN_USERS finding")
	}
}

func TestAuditUsers_SingleAdminNoFinding(t *testing.T) {
	users := []mongoinspect.UserInfo{
		{Username: "admin", Database: "admin", Roles: []mongoinspect.UserRole{
			{Role: "root", DB: "admin"},
		}},
	}

	findings := AuditUsers(users)
	for _, f := range findings {
		if f.Type == FindingMultipleAdminUsers {
			t.Error("single admin user should not trigger MULTIPLE_ADMIN_USERS")
		}
	}
}

func TestAuditUsers_NoFindings(t *testing.T) {
	users := []mongoinspect.UserInfo{
		{Username: "appUser", Database: "myapp", Roles: []mongoinspect.UserRole{
			{Role: "readWrite", DB: "myapp"},
		}},
	}

	findings := AuditUsers(users)
	if len(findings) != 0 {
		t.Errorf("expected no findings for normal user, got %d: %+v", len(findings), findings)
	}
}

func TestAuditUsers_Empty(t *testing.T) {
	findings := AuditUsers(nil)
	if len(findings) != 0 {
		t.Errorf("expected no findings for nil users, got %d", len(findings))
	}
}

func TestAuditUsers_AllAdminRoles(t *testing.T) {
	// Verify all adminRoles trigger ADMIN_IN_DATA_DB
	for role := range adminRoles {
		users := []mongoinspect.UserInfo{
			{Username: "testUser", Database: "myapp", Roles: []mongoinspect.UserRole{
				{Role: role, DB: "myapp"},
			}},
		}
		findings := detectAdminInDataDB(users)
		if len(findings) != 1 {
			t.Errorf("role %q should trigger ADMIN_IN_DATA_DB, got %d findings", role, len(findings))
		}
	}
}

func TestAuditUsers_AllClusterAdminRoles(t *testing.T) {
	// Verify all clusterAdminRoles trigger OVERPRIVILEGED_USER
	for role := range clusterAdminRoles {
		users := []mongoinspect.UserInfo{
			{Username: "testUser", Database: "admin", Roles: []mongoinspect.UserRole{
				{Role: role, DB: "admin"},
			}},
		}
		findings := detectOverprivilegedUsers(users)
		if len(findings) != 1 {
			t.Errorf("role %q should trigger OVERPRIVILEGED_USER, got %d findings", role, len(findings))
		}
	}
}

func TestAuditUsers_DuplicateUserNonAdminOnly(t *testing.T) {
	// Same username in two non-admin dbs should NOT trigger DUPLICATE_USER
	users := []mongoinspect.UserInfo{
		{Username: "shared", Database: "myapp", Roles: []mongoinspect.UserRole{
			{Role: "readWrite", DB: "myapp"},
		}},
		{Username: "shared", Database: "staging", Roles: []mongoinspect.UserRole{
			{Role: "readWrite", DB: "staging"},
		}},
	}

	findings := detectDuplicateUsers(users)
	if len(findings) != 0 {
		t.Errorf("duplicate in non-admin dbs should not trigger, got %d findings", len(findings))
	}
}

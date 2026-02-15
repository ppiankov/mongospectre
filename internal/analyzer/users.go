package analyzer

import (
	"fmt"
	"sort"
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

// adminRoles are roles that indicate administrative access,
// risky when found in non-admin databases.
var adminRoles = map[string]bool{
	"dbAdmin":   true,
	"dbOwner":   true,
	"root":      true,
	"userAdmin": true,
}

// clusterAdminRoles are cluster-wide admin roles.
var clusterAdminRoles = map[string]bool{
	"root":                 true,
	"clusterAdmin":         true,
	"userAdminAnyDatabase": true,
	"dbAdminAnyDatabase":   true,
}

// AuditUsers runs user-configuration detections against the given user list.
func AuditUsers(users []mongoinspect.UserInfo) []Finding {
	var findings []Finding
	findings = append(findings, detectAdminInDataDB(users)...)
	findings = append(findings, detectDuplicateUsers(users)...)
	findings = append(findings, detectOverprivilegedUsers(users)...)
	findings = append(findings, detectMultipleAdminUsers(users)...)
	return findings
}

// detectAdminInDataDB flags users with admin-level roles in non-admin databases.
func detectAdminInDataDB(users []mongoinspect.UserInfo) []Finding {
	var findings []Finding
	for _, u := range users {
		if u.Database == "admin" {
			continue
		}
		for _, r := range u.Roles {
			if adminRoles[r.Role] {
				findings = append(findings, Finding{
					Type:     FindingAdminInDataDB,
					Severity: SeverityHigh,
					Database: u.Database,
					Message:  fmt.Sprintf("user %q has admin role %q in non-admin database", u.Username, r.Role),
				})
				break
			}
		}
	}
	return findings
}

// detectDuplicateUsers flags usernames that exist in both admin and application databases.
func detectDuplicateUsers(users []mongoinspect.UserInfo) []Finding {
	byName := make(map[string][]string)
	for _, u := range users {
		byName[u.Username] = append(byName[u.Username], u.Database)
	}

	var findings []Finding
	for username, dbs := range byName {
		if len(dbs) < 2 {
			continue
		}
		hasAdmin := false
		var appDBs []string
		for _, db := range dbs {
			if db == "admin" {
				hasAdmin = true
			} else {
				appDBs = append(appDBs, db)
			}
		}
		if hasAdmin && len(appDBs) > 0 {
			sort.Strings(appDBs)
			findings = append(findings, Finding{
				Type:     FindingDuplicateUser,
				Severity: SeverityMedium,
				Database: "admin",
				Message:  fmt.Sprintf("user %q exists in admin and %s — auth source confusion risk", username, strings.Join(appDBs, ", ")),
			})
		}
	}
	return findings
}

// detectOverprivilegedUsers flags users with cluster-admin roles.
func detectOverprivilegedUsers(users []mongoinspect.UserInfo) []Finding {
	var findings []Finding
	for _, u := range users {
		var overRoles []string
		for _, r := range u.Roles {
			if clusterAdminRoles[r.Role] {
				overRoles = append(overRoles, r.Role)
			}
		}
		if len(overRoles) > 0 {
			findings = append(findings, Finding{
				Type:     FindingOverprivilegedUser,
				Severity: SeverityMedium,
				Database: u.Database,
				Message:  fmt.Sprintf("user %q has broad privileges: %s", u.Username, strings.Join(overRoles, ", ")),
			})
		}
	}
	return findings
}

// detectMultipleAdminUsers flags when more than one user has cluster-admin roles.
func detectMultipleAdminUsers(users []mongoinspect.UserInfo) []Finding {
	var adminUsers []string
	seen := make(map[string]bool)
	for _, u := range users {
		for _, r := range u.Roles {
			if clusterAdminRoles[r.Role] && !seen[u.Username] {
				adminUsers = append(adminUsers, u.Username)
				seen[u.Username] = true
			}
		}
	}
	if len(adminUsers) <= 1 {
		return nil
	}
	sort.Strings(adminUsers)
	return []Finding{{
		Type:     FindingMultipleAdminUsers,
		Severity: SeverityMedium,
		Database: "admin",
		Message:  fmt.Sprintf("%d users have cluster-admin roles: %s", len(adminUsers), strings.Join(adminUsers, ", ")),
	}}
}

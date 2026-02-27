package analyzer

import (
	"fmt"

	"github.com/ppiankov/mongospectre/internal/atlas"
)

// DetectInactiveUsers cross-references Atlas database users against access
// logs to identify users with no recent authentication activity.
// Atlas retains access logs for 7 days.
func DetectInactiveUsers(users []atlas.DatabaseUser, logs []atlas.AccessLogEntry) []Finding {
	if len(users) == 0 {
		return nil
	}

	successfulUsers := make(map[string]bool)
	failedUsers := make(map[string]bool)

	for _, entry := range logs {
		if entry.Username == "" {
			continue
		}
		if entry.AuthResult {
			successfulUsers[entry.Username] = true
		} else {
			failedUsers[entry.Username] = true
		}
	}

	var findings []Finding
	for _, u := range users {
		if successfulUsers[u.Username] {
			continue
		}

		if failedUsers[u.Username] {
			findings = append(findings, Finding{
				Type:     FindingFailedAuthOnly,
				Severity: SeverityMedium,
				Database: u.DatabaseName,
				Message:  fmt.Sprintf("user %q has only failed authentication attempts in the last 7 days", u.Username),
			})
			continue
		}

		if isPrivilegedAtlasUser(u) {
			findings = append(findings, Finding{
				Type:     FindingInactivePrivilegedUser,
				Severity: SeverityHigh,
				Database: u.DatabaseName,
				Message:  fmt.Sprintf("privileged user %q has no authentication in the last 7 days", u.Username),
			})
			continue
		}

		findings = append(findings, Finding{
			Type:     FindingInactiveUser,
			Severity: SeverityMedium,
			Database: u.DatabaseName,
			Message:  fmt.Sprintf("user %q has no authentication in the last 7 days", u.Username),
		})
	}
	return findings
}

func isPrivilegedAtlasUser(u atlas.DatabaseUser) bool {
	for _, r := range u.Roles {
		if clusterAdminRoles[r.RoleName] || r.RoleName == "readWriteAnyDatabase" {
			return true
		}
	}
	return false
}

package analyzer

import (
	"fmt"

	"github.com/ppiankov/mongospectre/internal/atlas"
)

// AuditAtlasUsers runs Atlas-specific user detections that require data
// only available from the Atlas Admin API (e.g., cluster scopes).
func AuditAtlasUsers(users []atlas.DatabaseUser) []Finding {
	var findings []Finding
	findings = append(findings, detectNoScope(users)...)
	return findings
}

// detectNoScope flags Atlas database users with no cluster scope restriction,
// meaning they have access to all clusters in the project.
func detectNoScope(users []atlas.DatabaseUser) []Finding {
	var findings []Finding
	for _, u := range users {
		if len(u.Scopes) == 0 && len(u.Roles) > 0 {
			findings = append(findings, Finding{
				Type:     FindingAtlasUserNoScope,
				Severity: SeverityInfo,
				Database: u.DatabaseName,
				Message:  fmt.Sprintf("Atlas user %q has no cluster scope restriction — access to all clusters in project", u.Username),
			})
		}
	}
	return findings
}

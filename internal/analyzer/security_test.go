package analyzer

import (
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func TestAuditSecurity_AuthDisabled(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled: false,
		TLSMode:     "requireTLS",
		BindIP:      "127.0.0.1",
	}
	findings := AuditSecurity(info)
	found := false
	for _, f := range findings {
		if f.Type == FindingAuthDisabled {
			found = true
			if f.Severity != SeverityHigh {
				t.Errorf("severity = %s, want high", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected AUTH_DISABLED finding")
	}
}

func TestAuditSecurity_BindAll(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled: true,
		TLSMode:     "requireTLS",
		BindIP:      "0.0.0.0",
	}
	findings := AuditSecurity(info)
	found := false
	for _, f := range findings {
		if f.Type == FindingBindAllInterfaces {
			found = true
			if f.Severity != SeverityHigh {
				t.Errorf("severity = %s, want high", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected BIND_ALL_INTERFACES finding")
	}
}

func TestAuditSecurity_TLSDisabled(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled: true,
		TLSMode:     "disabled",
		BindIP:      "127.0.0.1",
	}
	findings := AuditSecurity(info)
	found := false
	for _, f := range findings {
		if f.Type == FindingTLSDisabled {
			found = true
			if f.Severity != SeverityHigh {
				t.Errorf("severity = %s, want high", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected TLS_DISABLED finding")
	}
}

func TestAuditSecurity_TLSInvalidCerts(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled:          true,
		TLSMode:              "requireTLS",
		TLSAllowInvalidCerts: true,
		BindIP:               "127.0.0.1",
		AuditLogEnabled:      true,
	}
	findings := AuditSecurity(info)
	found := false
	for _, f := range findings {
		if f.Type == FindingTLSAllowInvalidCerts {
			found = true
			if f.Severity != SeverityMedium {
				t.Errorf("severity = %s, want medium", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected TLS_ALLOW_INVALID_CERTS finding")
	}
}

func TestAuditSecurity_AuditLogDisabled(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled:     true,
		TLSMode:         "requireTLS",
		BindIP:          "127.0.0.1",
		AuditLogEnabled: false,
	}
	findings := AuditSecurity(info)
	found := false
	for _, f := range findings {
		if f.Type == FindingAuditLogDisabled {
			found = true
			if f.Severity != SeverityMedium {
				t.Errorf("severity = %s, want medium", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected AUDIT_LOG_DISABLED finding")
	}
}

func TestAuditSecurity_LocalhostException(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled:         true,
		TLSMode:             "requireTLS",
		BindIP:              "127.0.0.1",
		AuditLogEnabled:     true,
		LocalhostAuthBypass: true,
	}
	findings := AuditSecurity(info)
	found := false
	for _, f := range findings {
		if f.Type == FindingLocalhostException {
			found = true
			if f.Severity != SeverityLow {
				t.Errorf("severity = %s, want low", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected LOCALHOST_EXCEPTION_ACTIVE finding")
	}
}

func TestAuditSecurity_FullyHardened(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled:          true,
		TLSMode:              "requireTLS",
		TLSAllowInvalidCerts: false,
		BindIP:               "127.0.0.1",
		AuditLogEnabled:      true,
		LocalhostAuthBypass:  false,
	}
	findings := AuditSecurity(info)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for fully hardened server, got %d", len(findings))
		for _, f := range findings {
			t.Errorf("  %s: %s", f.Type, f.Message)
		}
	}
}

func TestAuditSecurity_BindLocalhost(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled: true,
		TLSMode:     "requireTLS",
		BindIP:      "127.0.0.1",
	}
	findings := AuditSecurity(info)
	for _, f := range findings {
		if f.Type == FindingBindAllInterfaces {
			t.Error("should not flag BIND_ALL_INTERFACES for 127.0.0.1")
		}
	}
}

func TestAuditSecurity_BindIPv6All(t *testing.T) {
	info := mongoinspect.SecurityInfo{
		AuthEnabled: true,
		TLSMode:     "requireTLS",
		BindIP:      "::",
	}
	findings := AuditSecurity(info)
	found := false
	for _, f := range findings {
		if f.Type == FindingBindAllInterfaces {
			found = true
		}
	}
	if !found {
		t.Error("expected BIND_ALL_INTERFACES for IPv6 all-interfaces (::)")
	}
}

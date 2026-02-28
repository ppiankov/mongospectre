package analyzer

import (
	"strings"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

// AuditSecurity checks server security configuration for common misconfigurations.
func AuditSecurity(info mongoinspect.SecurityInfo) []Finding {
	var findings []Finding
	findings = append(findings, detectAuthDisabled(&info)...)
	findings = append(findings, detectBindAllInterfaces(&info)...)
	findings = append(findings, detectTLSDisabled(&info)...)
	findings = append(findings, detectTLSAllowInvalidCerts(&info)...)
	findings = append(findings, detectAuditLogDisabled(&info)...)
	findings = append(findings, detectLocalhostException(&info)...)
	return findings
}

func detectAuthDisabled(info *mongoinspect.SecurityInfo) []Finding {
	if info.AuthEnabled {
		return nil
	}
	return []Finding{{
		Type:     FindingAuthDisabled,
		Severity: SeverityHigh,
		Message:  "authentication is disabled — anyone can connect without credentials",
	}}
}

func detectBindAllInterfaces(info *mongoinspect.SecurityInfo) []Finding {
	if info.BindIP == "" {
		return nil
	}
	// Flag if bound to all interfaces.
	for _, addr := range strings.Split(info.BindIP, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "0.0.0.0" || addr == "::" {
			return []Finding{{
				Type:     FindingBindAllInterfaces,
				Severity: SeverityHigh,
				Message:  "server is bound to all network interfaces (" + addr + ") — restrict with net.bindIp",
			}}
		}
	}
	return nil
}

func detectTLSDisabled(info *mongoinspect.SecurityInfo) []Finding {
	mode := strings.ToLower(info.TLSMode)
	if mode == "" || mode == "disabled" {
		return []Finding{{
			Type:     FindingTLSDisabled,
			Severity: SeverityHigh,
			Message:  "TLS is not configured — network traffic is unencrypted",
		}}
	}
	return nil
}

func detectTLSAllowInvalidCerts(info *mongoinspect.SecurityInfo) []Finding {
	if !info.TLSAllowInvalidCerts {
		return nil
	}
	return []Finding{{
		Type:     FindingTLSAllowInvalidCerts,
		Severity: SeverityMedium,
		Message:  "tlsAllowInvalidCertificates is enabled — connections accept untrusted certificates (MITM risk)",
	}}
}

func detectAuditLogDisabled(info *mongoinspect.SecurityInfo) []Finding {
	if info.AuditLogEnabled {
		return nil
	}
	return []Finding{{
		Type:     FindingAuditLogDisabled,
		Severity: SeverityMedium,
		Message:  "audit logging is not configured — no trail of administrative actions",
	}}
}

func detectLocalhostException(info *mongoinspect.SecurityInfo) []Finding {
	if !info.LocalhostAuthBypass {
		return nil
	}
	return []Finding{{
		Type:     FindingLocalhostException,
		Severity: SeverityLow,
		Message:  "localhost authentication bypass is active — allows creating first user without credentials",
	}}
}

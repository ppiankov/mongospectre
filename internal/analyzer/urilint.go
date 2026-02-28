package analyzer

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	minConnectTimeoutMS         = 5000
	minServerSelectionTimeoutMS = 10000
	srvScheme                   = "mongodb+srv"
)

// LintURI performs static analysis on a MongoDB connection URI, returning
// findings for common misconfigurations. No MongoDB connection is needed.
func LintURI(rawURI string) []Finding {
	if rawURI == "" {
		return nil
	}

	// Detect SRV URIs — they have different TLS and discovery defaults.
	isSRV := strings.HasPrefix(rawURI, srvScheme+"://")

	u, err := url.Parse(rawURI)
	if err != nil {
		return []Finding{{
			Type:     FindingURINoAuth,
			Severity: SeverityLow,
			Message:  fmt.Sprintf("URI could not be parsed: %v", err),
		}}
	}

	var findings []Finding
	findings = append(findings, detectNoAuth(u, isSRV)...)
	findings = append(findings, detectNoTLS(u, isSRV)...)
	findings = append(findings, detectNoRetryWrites(u)...)
	findings = append(findings, detectPlaintextPassword(u)...)
	findings = append(findings, detectDefaultAuthSource(u)...)
	findings = append(findings, detectShortTimeout(u)...)
	findings = append(findings, detectNoReadPreference(u)...)
	findings = append(findings, detectDirectConnection(u, isSRV)...)
	return findings
}

// detectNoAuth flags URIs without credentials or an authMechanism on non-localhost hosts.
func detectNoAuth(u *url.URL, _ bool) []Finding {
	if isLocalhost(u.Hostname()) {
		return nil
	}
	hasUser := u.User != nil && u.User.Username() != ""
	hasAuthMechanism := u.Query().Get("authMechanism") != ""
	if hasUser || hasAuthMechanism {
		return nil
	}
	return []Finding{{
		Type:     FindingURINoAuth,
		Severity: SeverityLow,
		Message:  "URI has no credentials and no authMechanism — connection may be unauthenticated",
	}}
}

// detectNoTLS flags non-localhost, non-SRV URIs without tls=true or ssl=true.
func detectNoTLS(u *url.URL, isSRV bool) []Finding {
	if isLocalhost(u.Hostname()) {
		return nil
	}
	// SRV URIs use TLS by default.
	if isSRV {
		return nil
	}
	q := u.Query()
	if strings.EqualFold(q.Get("tls"), "true") || strings.EqualFold(q.Get("ssl"), "true") {
		return nil
	}
	return []Finding{{
		Type:     FindingURINoTLS,
		Severity: SeverityLow,
		Message:  "URI does not enable TLS — add ?tls=true for encrypted connections",
	}}
}

// detectNoRetryWrites flags URIs without retryWrites=true.
func detectNoRetryWrites(u *url.URL) []Finding {
	q := u.Query()
	if strings.EqualFold(q.Get("retryWrites"), "true") {
		return nil
	}
	return []Finding{{
		Type:     FindingURINoRetryWrites,
		Severity: SeverityInfo,
		Message:  "URI does not set retryWrites=true — older drivers default to false",
	}}
}

// detectPlaintextPassword flags URIs with an embedded password.
func detectPlaintextPassword(u *url.URL) []Finding {
	if u.User == nil {
		return nil
	}
	_, hasPassword := u.User.Password()
	if !hasPassword {
		return nil
	}
	return []Finding{{
		Type:     FindingURIPlaintextPassword,
		Severity: SeverityInfo,
		Message:  "URI contains an embedded password — consider using environment variables or a secrets manager",
	}}
}

// detectDefaultAuthSource flags URIs with credentials but no authSource parameter.
func detectDefaultAuthSource(u *url.URL) []Finding {
	if u.User == nil || u.User.Username() == "" {
		return nil
	}
	if u.Query().Get("authSource") != "" {
		return nil
	}
	return []Finding{{
		Type:     FindingURIDefaultAuthSource,
		Severity: SeverityInfo,
		Message:  "URI does not specify authSource — defaults to 'admin', which may be incorrect for application databases",
	}}
}

// detectShortTimeout flags connect or server selection timeouts that are too aggressive.
func detectShortTimeout(u *url.URL) []Finding {
	var findings []Finding
	q := u.Query()

	if v := q.Get("connectTimeoutMS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err == nil && ms < minConnectTimeoutMS {
			findings = append(findings, Finding{
				Type:     FindingURIShortTimeout,
				Severity: SeverityLow,
				Message:  fmt.Sprintf("connectTimeoutMS=%d is below %dms — may cause spurious timeouts in cloud environments", ms, minConnectTimeoutMS),
			})
		}
	}

	if v := q.Get("serverSelectionTimeoutMS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err == nil && ms < minServerSelectionTimeoutMS {
			findings = append(findings, Finding{
				Type:     FindingURIShortTimeout,
				Severity: SeverityLow,
				Message:  fmt.Sprintf("serverSelectionTimeoutMS=%d is below %dms — may cause connection failures during failover", ms, minServerSelectionTimeoutMS),
			})
		}
	}

	return findings
}

// detectNoReadPreference flags URIs without an explicit readPreference.
func detectNoReadPreference(u *url.URL) []Finding {
	if u.Query().Get("readPreference") != "" {
		return nil
	}
	return []Finding{{
		Type:     FindingURINoReadPreference,
		Severity: SeverityInfo,
		Message:  "URI does not set readPreference — defaults to 'primary', which may not suit read-heavy workloads",
	}}
}

// detectDirectConnection flags directConnection=true on SRV or multi-host URIs.
func detectDirectConnection(u *url.URL, isSRV bool) []Finding {
	if !strings.EqualFold(u.Query().Get("directConnection"), "true") {
		return nil
	}

	if isSRV {
		return []Finding{{
			Type:     FindingURIDirectConnection,
			Severity: SeverityLow,
			Message:  "directConnection=true is incompatible with SRV URIs — remove it to enable SRV discovery",
		}}
	}

	// Check for multiple hosts (comma-separated in the host portion).
	if strings.Contains(u.Host, ",") {
		return []Finding{{
			Type:     FindingURIDirectConnection,
			Severity: SeverityLow,
			Message:  "directConnection=true with multiple hosts bypasses replica set failover",
		}}
	}

	return nil
}

// isLocalhost returns true for development/local addresses.
func isLocalhost(host string) bool {
	h := strings.ToLower(host)
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

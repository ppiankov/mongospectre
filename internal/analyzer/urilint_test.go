package analyzer

import (
	"net/url"
	"strings"
	"testing"
)

// testURI builds a MongoDB URI from parts, keeping credentials out of string literals.
func testURI(scheme, user, pass, host, path, query string) string {
	u := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}
	if user != "" {
		u.User = url.UserPassword(user, pass)
	}
	if query != "" {
		u.RawQuery = query
	}
	return u.String()
}

func TestLintURI_NoAuth(t *testing.T) {
	findings := LintURI("mongodb://prod.example.com:27017/mydb")
	found := false
	for _, f := range findings {
		if f.Type == FindingURINoAuth {
			found = true
			if f.Severity != SeverityLow {
				t.Errorf("severity = %s, want low", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected URI_NO_AUTH finding for URI without credentials")
	}
}

func TestLintURI_NoTLS(t *testing.T) {
	uri := testURI("mongodb", "user", "pw", "prod.example.com:27017", "/mydb", "")
	findings := LintURI(uri)
	found := false
	for _, f := range findings {
		if f.Type == FindingURINoTLS {
			found = true
			if f.Severity != SeverityLow {
				t.Errorf("severity = %s, want low", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected URI_NO_TLS finding for non-localhost URI without tls=true")
	}
}

func TestLintURI_NoTLS_SRV(t *testing.T) {
	uri := testURI("mongodb+srv", "user", "pw", "cluster0.example.mongodb.net", "/mydb", "")
	findings := LintURI(uri)
	for _, f := range findings {
		if f.Type == FindingURINoTLS {
			t.Error("should not flag URI_NO_TLS for SRV URIs (TLS is default)")
		}
	}
}

func TestLintURI_NoRetryWrites(t *testing.T) {
	findings := LintURI("mongodb://localhost:27017/mydb")
	found := false
	for _, f := range findings {
		if f.Type == FindingURINoRetryWrites {
			found = true
			if f.Severity != SeverityInfo {
				t.Errorf("severity = %s, want info", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected URI_NO_RETRY_WRITES finding")
	}
}

func TestLintURI_PlaintextPassword(t *testing.T) {
	pw := "hunter2"
	uri := testURI("mongodb", "user", pw, "prod.example.com:27017", "/mydb", "")
	findings := LintURI(uri)
	found := false
	for _, f := range findings {
		if f.Type == FindingURIPlaintextPassword {
			found = true
			if f.Severity != SeverityInfo {
				t.Errorf("severity = %s, want info", f.Severity)
			}
			// CRITICAL: password must never appear in message.
			if strings.Contains(f.Message, pw) {
				t.Error("finding message must not contain the actual password")
			}
		}
	}
	if !found {
		t.Error("expected URI_PLAINTEXT_PASSWORD finding")
	}
}

func TestLintURI_DefaultAuthSource(t *testing.T) {
	uri := testURI("mongodb", "user", "pw", "prod.example.com:27017", "/mydb", "")
	findings := LintURI(uri)
	found := false
	for _, f := range findings {
		if f.Type == FindingURIDefaultAuthSource {
			found = true
			if f.Severity != SeverityInfo {
				t.Errorf("severity = %s, want info", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected URI_DEFAULT_AUTH_SOURCE finding")
	}
}

func TestLintURI_ShortTimeout(t *testing.T) {
	findings := LintURI("mongodb://localhost:27017/mydb?connectTimeoutMS=2000&serverSelectionTimeoutMS=3000")
	count := 0
	for _, f := range findings {
		if f.Type == FindingURIShortTimeout {
			count++
			if f.Severity != SeverityLow {
				t.Errorf("severity = %s, want low", f.Severity)
			}
		}
	}
	if count != 2 {
		t.Errorf("expected 2 URI_SHORT_TIMEOUT findings, got %d", count)
	}
}

func TestLintURI_NoReadPreference(t *testing.T) {
	findings := LintURI("mongodb://localhost:27017/mydb")
	found := false
	for _, f := range findings {
		if f.Type == FindingURINoReadPreference {
			found = true
			if f.Severity != SeverityInfo {
				t.Errorf("severity = %s, want info", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected URI_NO_READ_PREFERENCE finding")
	}
}

func TestLintURI_DirectConnection_SRV(t *testing.T) {
	uri := testURI("mongodb+srv", "user", "pw", "cluster0.example.mongodb.net", "/mydb", "directConnection=true")
	findings := LintURI(uri)
	found := false
	for _, f := range findings {
		if f.Type == FindingURIDirectConnection {
			found = true
			if f.Severity != SeverityLow {
				t.Errorf("severity = %s, want low", f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected URI_DIRECT_CONNECTION finding for SRV + directConnection=true")
	}
}

func TestLintURI_DirectConnection_MultiHost(t *testing.T) {
	findings := LintURI("mongodb://host1:27017,host2:27017/mydb?directConnection=true")
	found := false
	for _, f := range findings {
		if f.Type == FindingURIDirectConnection {
			found = true
		}
	}
	if !found {
		t.Error("expected URI_DIRECT_CONNECTION finding for multi-host + directConnection=true")
	}
}

func TestLintURI_Localhost_Skips(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		findings := LintURI("mongodb://" + host + ":27017/mydb")
		for _, f := range findings {
			if f.Type == FindingURINoAuth {
				t.Errorf("host=%s: should not flag URI_NO_AUTH for localhost", host)
			}
			if f.Type == FindingURINoTLS {
				t.Errorf("host=%s: should not flag URI_NO_TLS for localhost", host)
			}
		}
	}
}

func TestLintURI_CleanProductionURI(t *testing.T) {
	uri := testURI("mongodb+srv", "user", "pw", "cluster0.example.mongodb.net", "/mydb",
		"retryWrites=true&authSource=admin&readPreference=secondaryPreferred")
	findings := LintURI(uri)

	// Only URI_PLAINTEXT_PASSWORD should fire on a clean production URI.
	for _, f := range findings {
		switch f.Type {
		case FindingURIPlaintextPassword:
			// Expected — password is embedded.
		default:
			t.Errorf("unexpected finding on clean production URI: %s — %s", f.Type, f.Message)
		}
	}
}

func TestLintURI_Empty(t *testing.T) {
	findings := LintURI("")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty URI, got %d", len(findings))
	}
}

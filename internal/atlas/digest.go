package atlas

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

type digestChallenge struct {
	Realm     string
	Nonce     string
	Opaque    string
	QOP       string
	Algorithm string
}

// digestTransport performs HTTP Digest auth for Atlas API key auth.
type digestTransport struct {
	username string
	password string
	rt       http.RoundTripper

	mu         sync.Mutex
	challenge  *digestChallenge
	nonceCount uint32
}

func newDigestTransport(username, password string, base http.RoundTripper) *digestTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &digestTransport{
		username: username,
		password: password,
		rt:       base,
	}
}

func (t *digestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	if chal, ok := t.getChallenge(); ok {
		res, err := t.sendWithChallenge(req, chal)
		if err != nil || res.StatusCode != http.StatusUnauthorized {
			return res, err
		}
		return t.handleUnauthorized(req, res)
	}

	res, err := t.rt.RoundTrip(req)
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		return res, err
	}
	return t.handleUnauthorized(req, res)
}

func (t *digestTransport) handleUnauthorized(req *http.Request, res *http.Response) (*http.Response, error) {
	chal, ok := parseDigestChallenge(res.Header.Get("WWW-Authenticate"))
	if !ok {
		return res, nil
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()

	t.storeChallenge(chal)
	return t.sendWithChallenge(req, chal)
}

func (t *digestTransport) sendWithChallenge(req *http.Request, chal *digestChallenge) (*http.Response, error) {
	req2 := cloneRequest(req)
	nc := t.nextNonceCount()
	req2.Header.Set("Authorization", buildDigestAuthHeader(req2, chal, t.username, t.password, nc))
	return t.rt.RoundTrip(req2)
}

func (t *digestTransport) getChallenge() (*digestChallenge, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.challenge == nil {
		return nil, false
	}
	c := *t.challenge
	return &c, true
}

func (t *digestTransport) storeChallenge(chal *digestChallenge) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if chal == nil {
		t.challenge = nil
	} else {
		c := *chal
		t.challenge = &c
	}
	t.nonceCount = 0
}

func (t *digestTransport) nextNonceCount() uint32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nonceCount++
	return t.nonceCount
}

func cloneRequest(req *http.Request) *http.Request {
	req2 := req.Clone(req.Context())
	req2.Header = req.Header.Clone()
	return req2
}

func buildDigestAuthHeader(req *http.Request, chal *digestChallenge, username, password string, nc uint32) string {
	if chal == nil {
		return ""
	}

	uri := req.URL.RequestURI()
	method := req.Method
	cnonce := randomHex(8)
	if cnonce == "" {
		cnonce = "cafebabe"
	}

	algorithm := strings.ToUpper(strings.TrimSpace(chal.Algorithm))
	if algorithm == "" {
		algorithm = "MD5"
	}
	qop := pickQOP(chal.QOP)

	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", username, chal.Realm, password))
	ha2 := md5Hex(fmt.Sprintf("%s:%s", method, uri))

	var response string
	if qop != "" {
		response = md5Hex(fmt.Sprintf("%s:%s:%08x:%s:%s:%s", ha1, chal.Nonce, nc, cnonce, qop, ha2))
	} else {
		response = md5Hex(fmt.Sprintf("%s:%s:%s", ha1, chal.Nonce, ha2))
	}

	parts := []string{
		`Digest username="` + escapeDigestValue(username) + `"`,
		`realm="` + escapeDigestValue(chal.Realm) + `"`,
		`nonce="` + escapeDigestValue(chal.Nonce) + `"`,
		`uri="` + escapeDigestValue(uri) + `"`,
		`response="` + response + `"`,
	}
	if chal.Opaque != "" {
		parts = append(parts, `opaque="`+escapeDigestValue(chal.Opaque)+`"`)
	}
	if algorithm != "" {
		parts = append(parts, "algorithm="+algorithm)
	}
	if qop != "" {
		parts = append(parts, "qop="+qop, fmt.Sprintf("nc=%08x", nc), `cnonce="`+cnonce+`"`)
	}
	return strings.Join(parts, ", ")
}

func parseDigestChallenge(header string) (*digestChallenge, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil, false
	}

	prefix := "Digest "
	if !strings.HasPrefix(strings.ToLower(header), strings.ToLower(prefix)) {
		return nil, false
	}
	params := parseAuthParams(header[len(prefix):])
	chal := &digestChallenge{
		Realm:     params["realm"],
		Nonce:     params["nonce"],
		Opaque:    params["opaque"],
		QOP:       params["qop"],
		Algorithm: params["algorithm"],
	}
	if chal.Realm == "" || chal.Nonce == "" {
		return nil, false
	}
	return chal, true
}

func parseAuthParams(input string) map[string]string {
	parts := splitAuthHeader(input)
	out := make(map[string]string, len(parts))
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])
		val = strings.Trim(val, `"`)
		if key != "" {
			out[key] = val
		}
	}
	return out
}

func splitAuthHeader(input string) []string {
	var out []string
	start := 0
	inQuotes := false
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if inQuotes {
				continue
			}
			out = append(out, strings.TrimSpace(input[start:i]))
			start = i + 1
		}
	}
	if start < len(input) {
		out = append(out, strings.TrimSpace(input[start:]))
	}
	return out
}

func pickQOP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		if strings.EqualFold(strings.TrimSpace(p), "auth") {
			return "auth"
		}
	}
	return strings.TrimSpace(parts[0])
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func escapeDigestValue(v string) string {
	v = strings.ReplaceAll(v, `\\`, `\\\\`)
	v = strings.ReplaceAll(v, `"`, `\\"`)
	return v
}

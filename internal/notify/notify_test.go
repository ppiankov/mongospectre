package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	"github.com/ppiankov/mongospectre/internal/config"
)

type capturedRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

type recordingRoundTripper struct {
	mu       sync.Mutex
	requests []capturedRequest
	status   int
	respBody string
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()

	r.mu.Lock()
	r.requests = append(r.requests, capturedRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: req.Header.Clone(),
		Body:    body,
	})
	r.mu.Unlock()

	status := r.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(r.respBody)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (r *recordingRoundTripper) snapshot() []capturedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]capturedRequest, len(r.requests))
	copy(out, r.requests)
	return out
}

func TestEventsFromDiff(t *testing.T) {
	diff := []analyzer.BaselineFinding{
		{
			Finding: analyzer.Finding{Type: analyzer.FindingMissingIndex, Severity: analyzer.SeverityHigh},
			Status:  analyzer.StatusNew,
		},
		{
			Finding: analyzer.Finding{Type: analyzer.FindingUnusedIndex, Severity: analyzer.SeverityMedium},
			Status:  analyzer.StatusNew,
		},
		{
			Finding: analyzer.Finding{Type: analyzer.FindingMissingTTL, Severity: analyzer.SeverityLow},
			Status:  analyzer.StatusNew,
		},
		{
			Finding: analyzer.Finding{Type: analyzer.FindingOK, Severity: analyzer.SeverityInfo},
			Status:  analyzer.StatusNew,
		},
		{
			Finding: analyzer.Finding{Type: analyzer.FindingUnusedCollection, Severity: analyzer.SeverityMedium},
			Status:  analyzer.StatusResolved,
		},
	}

	events := EventsFromDiff(diff, time.Date(2026, 2, 17, 21, 0, 0, 0, time.UTC))
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4", len(events))
	}

	gotTypes := []EventType{events[0].Type, events[1].Type, events[2].Type, events[3].Type}
	wantTypes := []EventType{EventNewHigh, EventNewMedium, EventNewLow, EventResolved}
	for i := range wantTypes {
		if gotTypes[i] != wantTypes[i] {
			t.Fatalf("event[%d] = %s, want %s", i, gotTypes[i], wantTypes[i])
		}
	}
}

func TestNewDispatcherExpandsEnvPlaceholders(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/123")
	t.Setenv("ALERT_TOKEN", "abc123")

	d, err := NewDispatcher([]config.Notification{
		{
			Type:       "slack",
			WebhookURL: "${SLACK_WEBHOOK_URL}",
			On:         []string{"new_high"},
		},
		{
			Type:   "webhook",
			URL:    "https://alerts.test/v1",
			Method: "post",
			Headers: map[string]string{
				"Authorization": "Bearer ${ALERT_TOKEN}",
			},
		},
	}, DispatcherOptions{})
	if err != nil {
		t.Fatalf("NewDispatcher error: %v", err)
	}

	if got := d.channels[0].slack.webhookURL; got != "https://hooks.slack.test/123" {
		t.Fatalf("slack webhook URL = %q", got)
	}
	if got := d.channels[1].webhook.headers["Authorization"]; got != "Bearer abc123" {
		t.Fatalf("authorization header = %q", got)
	}
	if got := d.channels[1].webhook.method; got != http.MethodPost {
		t.Fatalf("webhook method = %q", got)
	}
}

func TestNewDispatcherRejectsPlaintextSecrets(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Notification
		want string
	}{
		{
			name: "slack webhook",
			cfg: config.Notification{
				Type:       "slack",
				WebhookURL: "https://hooks.slack.test/plaintext",
			},
			want: "slack webhook_url must use ${ENV_VAR} placeholder",
		},
		{
			name: "webhook auth header",
			cfg: config.Notification{
				Type: "webhook",
				URL:  "https://alerts.example.com/hook",
				Headers: map[string]string{
					"Authorization": "Bearer token",
				},
			},
			want: `webhook header "Authorization" must use ${ENV_VAR} placeholder`,
		},
		{
			name: "email smtp password",
			cfg: config.Notification{
				Type:         "email",
				SMTPHost:     "smtp.example.com",
				SMTPPassword: "plain-secret",
				To:           []string{"team@example.com"},
			},
			want: "email smtp_password must use ${ENV_VAR} placeholder",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDispatcher([]config.Notification{tt.cfg}, DispatcherOptions{})
			if err == nil {
				t.Fatal("expected error for plaintext secret")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestNewDispatcherRejectsUnsetSecretEnv(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "")

	_, err := NewDispatcher([]config.Notification{
		{
			Type:       "slack",
			WebhookURL: "${SLACK_WEBHOOK_URL}",
		},
	}, DispatcherOptions{})
	if err == nil {
		t.Fatal("expected error for unset secret env var")
	}
	if !strings.Contains(err.Error(), `references unset env var "SLACK_WEBHOOK_URL"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewDispatcherRequiresEmailPasswordWhenUsernameIsSet(t *testing.T) {
	_, err := NewDispatcher([]config.Notification{
		{
			Type:         "email",
			SMTPHost:     "smtp.example.com",
			SMTPUsername: "mailer",
			To:           []string{"team@example.com"},
		},
	}, DispatcherOptions{})
	if err == nil {
		t.Fatal("expected error when smtp_username is set without smtp_password")
	}
	if !strings.Contains(err.Error(), "smtp_password is required when smtp_username is set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDispatcherNotifySlackAndWebhook(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/slack")
	t.Setenv("ALERT_TOKEN", "token")

	rt := &recordingRoundTripper{}
	httpClient := &http.Client{Transport: rt}

	d, err := NewDispatcher([]config.Notification{
		{
			Type:       "slack",
			WebhookURL: "${SLACK_WEBHOOK_URL}",
			On:         []string{"new_high"},
		},
		{
			Type:   "webhook",
			URL:    "https://alerts.example.com/hook",
			Method: "POST",
			Headers: map[string]string{
				"Authorization": "Bearer ${ALERT_TOKEN}",
			},
			On: []string{"new_high"},
		},
	}, DispatcherOptions{HTTPClient: httpClient, Interval: time.Minute})
	if err != nil {
		t.Fatalf("NewDispatcher error: %v", err)
	}

	event := Event{
		Type:      EventNewHigh,
		Timestamp: "2026-02-17T21:30:00Z",
		Status:    analyzer.StatusNew,
		Finding: analyzer.Finding{
			Type:       analyzer.FindingMissingIndex,
			Severity:   analyzer.SeverityHigh,
			Database:   "app",
			Collection: "orders",
			Message:    "missing index",
		},
	}

	if err := d.Notify(context.Background(), []Event{event}); err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	requests := rt.snapshot()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}

	if requests[0].URL != "https://hooks.slack.test/slack" {
		t.Fatalf("slack request URL = %q", requests[0].URL)
	}
	if requests[1].URL != "https://alerts.example.com/hook" {
		t.Fatalf("webhook request URL = %q", requests[1].URL)
	}
	if got := requests[1].Headers.Get("Authorization"); got != "Bearer token" {
		t.Fatalf("authorization header = %q", got)
	}
}

func TestDispatcherRateLimiting(t *testing.T) {
	rt := &recordingRoundTripper{}
	httpClient := &http.Client{Transport: rt}

	now := time.Date(2026, 2, 17, 22, 0, 0, 0, time.UTC)
	d, err := NewDispatcher([]config.Notification{
		{Type: "webhook", URL: "https://alerts.example.com/hook", On: []string{"new_high"}},
	}, DispatcherOptions{
		HTTPClient: httpClient,
		Interval:   time.Minute,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("NewDispatcher error: %v", err)
	}

	event := Event{
		Type:      EventNewHigh,
		Timestamp: now.Format(time.RFC3339),
		Status:    analyzer.StatusNew,
		Finding: analyzer.Finding{
			Type:       analyzer.FindingMissingIndex,
			Severity:   analyzer.SeverityHigh,
			Database:   "app",
			Collection: "orders",
		},
	}

	if err := d.Notify(context.Background(), []Event{event}); err != nil {
		t.Fatalf("first Notify error: %v", err)
	}
	if err := d.Notify(context.Background(), []Event{event}); err != nil {
		t.Fatalf("second Notify error: %v", err)
	}
	now = now.Add(30 * time.Second)
	if err := d.Notify(context.Background(), []Event{event}); err != nil {
		t.Fatalf("third Notify error: %v", err)
	}
	now = now.Add(31 * time.Second)
	if err := d.Notify(context.Background(), []Event{event}); err != nil {
		t.Fatalf("fourth Notify error: %v", err)
	}

	requests := rt.snapshot()
	if len(requests) != 2 {
		t.Fatalf("webhook calls = %d, want 2", len(requests))
	}
}

func TestDispatcherDryRunLogsPayloadWithoutSending(t *testing.T) {
	rt := &recordingRoundTripper{}
	httpClient := &http.Client{Transport: rt}

	var logs bytes.Buffer
	d, err := NewDispatcher([]config.Notification{
		{Type: "webhook", URL: "https://alerts.example.com/hook", On: []string{"new_medium"}},
	}, DispatcherOptions{
		DryRun:     true,
		Writer:     &logs,
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("NewDispatcher error: %v", err)
	}

	event := Event{
		Type:      EventNewMedium,
		Timestamp: "2026-02-17T22:10:00Z",
		Status:    analyzer.StatusNew,
		Finding: analyzer.Finding{
			Type:       analyzer.FindingUnusedIndex,
			Severity:   analyzer.SeverityMedium,
			Database:   "app",
			Collection: "users",
			Index:      "email_1",
			Message:    "unused index",
		},
	}

	if err := d.Notify(context.Background(), []Event{event}); err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	if len(rt.snapshot()) != 0 {
		t.Fatalf("expected no outbound requests in dry-run, got %d", len(rt.snapshot()))
	}
	if !strings.Contains(logs.String(), "[notify dry-run]") {
		t.Fatalf("missing dry-run log: %s", logs.String())
	}
}

func TestDispatcherEmailSend(t *testing.T) {
	called := false
	addr := ""
	from := ""
	var to []string
	var raw []byte

	d, err := NewDispatcher([]config.Notification{
		{
			Type:     "email",
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
			From:     "alerts@example.com",
			To:       []string{"b@example.com", "a@example.com"},
			On:       []string{"new_low"},
		},
	}, DispatcherOptions{
		SendMail: func(a string, _ smtp.Auth, f string, t []string, msg []byte) error {
			called = true
			addr = a
			from = f
			to = append([]string(nil), t...)
			raw = append([]byte(nil), msg...)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewDispatcher error: %v", err)
	}

	event := Event{
		Type:      EventNewLow,
		Timestamp: "2026-02-17T22:15:00Z",
		Status:    analyzer.StatusNew,
		Finding: analyzer.Finding{
			Type:       analyzer.FindingMissingTTL,
			Severity:   analyzer.SeverityLow,
			Database:   "app",
			Collection: "sessions",
			Message:    "missing ttl",
		},
	}

	if err := d.Notify(context.Background(), []Event{event}); err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	if !called {
		t.Fatal("expected sendMail to be called")
	}
	if addr != "smtp.example.com:587" {
		t.Fatalf("addr = %q, want smtp.example.com:587", addr)
	}
	if from != "alerts@example.com" {
		t.Fatalf("from = %q, want alerts@example.com", from)
	}
	if len(to) != 2 || to[0] != "a@example.com" || to[1] != "b@example.com" {
		t.Fatalf("to = %v, want sorted recipients", to)
	}
	if !strings.Contains(string(raw), "Content-Type: text/html") {
		t.Fatalf("expected HTML email payload, got: %s", string(raw))
	}
}

func TestBuildWebhookPayloadDoesNotIncludeURI(t *testing.T) {
	payload, err := buildWebhookPayload(&Event{
		Type:      EventNewHigh,
		Timestamp: "2026-02-17T22:30:00Z",
		Status:    analyzer.StatusNew,
		Finding: analyzer.Finding{
			Type:       analyzer.FindingMissingIndex,
			Severity:   analyzer.SeverityHigh,
			Database:   "app",
			Collection: "orders",
			Message:    "missing index",
		},
	})
	if err != nil {
		t.Fatalf("buildWebhookPayload error: %v", err)
	}

	if strings.Contains(string(payload), "mongodb://") {
		t.Fatalf("payload should not contain MongoDB URI: %s", string(payload))
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("invalid payload JSON: %v", err)
	}
}

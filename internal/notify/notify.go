package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ppiankov/mongospectre/internal/analyzer"
	"github.com/ppiankov/mongospectre/internal/config"
)

// EventType identifies the notification trigger type.
type EventType string

const (
	EventNewHigh   EventType = "new_high"
	EventNewMedium EventType = "new_medium"
	EventNewLow    EventType = "new_low"
	EventResolved  EventType = "resolved"
)

var allEventTypes = []EventType{EventNewHigh, EventNewMedium, EventNewLow, EventResolved}

// Event is a single notification-ready drift change.
type Event struct {
	Type      EventType               `json:"type"`
	Timestamp string                  `json:"timestamp"`
	Finding   analyzer.Finding        `json:"finding"`
	Status    analyzer.BaselineStatus `json:"status"`
}

// EventsFromDiff converts baseline diff entries into notification events.
func EventsFromDiff(diff []analyzer.BaselineFinding, at time.Time) []Event {
	timestamp := at.UTC().Format(time.RFC3339)
	events := make([]Event, 0, len(diff))
	for i := range diff {
		item := &diff[i]
		eventType, ok := eventTypeForFinding(item)
		if !ok {
			continue
		}
		events = append(events, Event{
			Type:      eventType,
			Timestamp: timestamp,
			Finding:   item.Finding,
			Status:    item.Status,
		})
	}
	return events
}

func eventTypeForFinding(item *analyzer.BaselineFinding) (EventType, bool) {
	switch item.Status {
	case analyzer.StatusResolved:
		return EventResolved, true
	case analyzer.StatusNew:
		switch item.Severity {
		case analyzer.SeverityHigh:
			return EventNewHigh, true
		case analyzer.SeverityMedium:
			return EventNewMedium, true
		case analyzer.SeverityLow:
			return EventNewLow, true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

// DispatcherOptions configures the notification dispatcher.
type DispatcherOptions struct {
	Interval   time.Duration
	DryRun     bool
	Writer     io.Writer
	HTTPClient *http.Client
	Now        func() time.Time
	SendMail   func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error
}

// Dispatcher routes watch events to configured notification channels.
type Dispatcher struct {
	channels   []channel
	interval   time.Duration
	dryRun     bool
	writer     io.Writer
	httpClient *http.Client
	now        func() time.Time
	sendMail   func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error

	mu       sync.Mutex
	lastSent map[string]time.Time
}

type channelKind string

const (
	channelSlack   channelKind = "slack"
	channelWebhook channelKind = "webhook"
	channelEmail   channelKind = "email"
)

type channel struct {
	id   string
	kind channelKind
	on   map[EventType]bool

	slack   *slackChannel
	webhook *webhookChannel
	email   *emailChannel
}

type slackChannel struct {
	webhookURL   string
	dashboardURL string
}

type webhookChannel struct {
	url     string
	method  string
	headers map[string]string
}

type emailChannel struct {
	host     string
	port     int
	username string
	password string
	from     string
	to       []string
	subject  string
}

// NewDispatcher builds a dispatcher from config file notification entries.
func NewDispatcher(cfgs []config.Notification, opts DispatcherOptions) (*Dispatcher, error) {
	channels, err := buildChannels(cfgs)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("no notification channels configured")
	}

	writer := opts.Writer
	if writer == nil {
		writer = io.Discard
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}

	sendMail := opts.SendMail
	if sendMail == nil {
		sendMail = smtp.SendMail
	}

	return &Dispatcher{
		channels:   channels,
		interval:   opts.Interval,
		dryRun:     opts.DryRun,
		writer:     writer,
		httpClient: httpClient,
		now:        now,
		sendMail:   sendMail,
		lastSent:   make(map[string]time.Time),
	}, nil
}

// Notify sends all events to all matching channels and aggregates non-fatal send errors.
func (d *Dispatcher) Notify(ctx context.Context, events []Event) error {
	var sendErrs []error

	for i := range events {
		event := &events[i]
		for _, ch := range d.channels {
			if !ch.on[event.Type] {
				continue
			}

			key := rateLimitKey(ch.id, &event.Finding)
			if !d.allow(key) {
				continue
			}

			if err := d.sendEvent(ctx, ch, event); err != nil {
				sendErrs = append(sendErrs, fmt.Errorf("%s: %w", ch.id, err))
			}
		}
	}

	return errors.Join(sendErrs...)
}

func (d *Dispatcher) sendEvent(ctx context.Context, ch channel, event *Event) error {
	switch ch.kind {
	case channelSlack:
		payload, err := buildSlackPayload(event, ch.slack.dashboardURL)
		if err != nil {
			return err
		}
		if d.dryRun {
			d.logDryRun(ch.id, event.Type, payload)
			return nil
		}
		return d.postJSON(ctx, http.MethodPost, ch.slack.webhookURL, nil, payload)
	case channelWebhook:
		payload, err := buildWebhookPayload(event)
		if err != nil {
			return err
		}
		if d.dryRun {
			d.logDryRun(ch.id, event.Type, payload)
			return nil
		}
		return d.postJSON(ctx, ch.webhook.method, ch.webhook.url, ch.webhook.headers, payload)
	case channelEmail:
		subject, message := buildEmailMessage(event, ch.email)
		if d.dryRun {
			d.logEmailDryRun(ch.id, event.Type, subject, ch.email.to, message)
			return nil
		}
		addr := fmt.Sprintf("%s:%d", ch.email.host, ch.email.port)
		var auth smtp.Auth
		if ch.email.username != "" {
			auth = smtp.PlainAuth("", ch.email.username, ch.email.password, ch.email.host)
		}
		return d.sendMail(addr, auth, ch.email.from, ch.email.to, message)
	default:
		return fmt.Errorf("unsupported channel type: %s", ch.kind)
	}
}

func (d *Dispatcher) postJSON(ctx context.Context, method, url string, headers map[string]string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (d *Dispatcher) allow(key string) bool {
	if d.interval <= 0 {
		return true
	}

	now := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()

	if ts, ok := d.lastSent[key]; ok && now.Sub(ts) < d.interval {
		return false
	}

	d.lastSent[key] = now

	pruneBefore := now.Add(-2 * d.interval)
	for k, ts := range d.lastSent {
		if ts.Before(pruneBefore) {
			delete(d.lastSent, k)
		}
	}

	return true
}

func (d *Dispatcher) logDryRun(channelID string, eventType EventType, payload []byte) {
	_, _ = fmt.Fprintf(d.writer, "[notify dry-run] channel=%s event=%s payload=%s\n", channelID, eventType, string(payload))
}

func (d *Dispatcher) logEmailDryRun(channelID string, eventType EventType, subject string, to []string, message []byte) {
	preview := map[string]string{
		"subject": subject,
		"to":      strings.Join(to, ","),
		"message": string(message),
	}
	data, _ := json.Marshal(preview)
	_, _ = fmt.Fprintf(d.writer, "[notify dry-run] channel=%s event=%s payload=%s\n", channelID, eventType, string(data))
}

func buildChannels(cfgs []config.Notification) ([]channel, error) {
	channels := make([]channel, 0, len(cfgs))
	for i := range cfgs {
		raw := &cfgs[i]
		kind := channelKind(strings.ToLower(strings.TrimSpace(raw.Type)))
		on, err := parseEventFilters(raw.On)
		if err != nil {
			return nil, fmt.Errorf("notifications[%d]: %w", i, err)
		}

		switch kind {
		case channelSlack:
			webhookURL, err := resolveSecretFromEnv(raw.WebhookURL, "slack webhook_url")
			if err != nil {
				return nil, fmt.Errorf("notifications[%d]: %w", i, err)
			}
			channels = append(channels, channel{
				id:   fmt.Sprintf("slack[%d]", i),
				kind: channelSlack,
				on:   on,
				slack: &slackChannel{
					webhookURL:   webhookURL,
					dashboardURL: expandEnvPlaceholders(strings.TrimSpace(raw.DashboardURL)),
				},
			})
		case channelWebhook:
			url := expandEnvPlaceholders(strings.TrimSpace(raw.URL))
			if url == "" {
				return nil, fmt.Errorf("notifications[%d]: webhook url is required", i)
			}
			headers := make(map[string]string, len(raw.Headers))
			for k, v := range raw.Headers {
				if isSensitiveHeader(k) {
					resolved, err := resolveSecretFromEnv(v, fmt.Sprintf("webhook header %q", k))
					if err != nil {
						return nil, fmt.Errorf("notifications[%d]: %w", i, err)
					}
					headers[k] = resolved
					continue
				}
				headers[k] = expandEnvPlaceholders(v)
			}
			method := strings.ToUpper(strings.TrimSpace(raw.Method))
			if method == "" {
				method = http.MethodPost
			}
			channels = append(channels, channel{
				id:   fmt.Sprintf("webhook[%d]", i),
				kind: channelWebhook,
				on:   on,
				webhook: &webhookChannel{
					url:     url,
					method:  method,
					headers: headers,
				},
			})
		case channelEmail:
			host := expandEnvPlaceholders(strings.TrimSpace(raw.SMTPHost))
			if host == "" {
				return nil, fmt.Errorf("notifications[%d]: email smtp_host is required", i)
			}
			port := raw.SMTPPort
			if port == 0 {
				port = 25
			}

			to := make([]string, 0, len(raw.To))
			for _, recipient := range raw.To {
				recipient = strings.TrimSpace(expandEnvPlaceholders(recipient))
				if recipient != "" {
					to = append(to, recipient)
				}
			}
			if len(to) == 0 {
				return nil, fmt.Errorf("notifications[%d]: email to is required", i)
			}
			sort.Strings(to)

			from := strings.TrimSpace(expandEnvPlaceholders(raw.From))
			if from == "" {
				from = "mongospectre@localhost"
			}
			password := ""
			if strings.TrimSpace(raw.SMTPPassword) != "" {
				password, err = resolveSecretFromEnv(raw.SMTPPassword, "email smtp_password")
				if err != nil {
					return nil, fmt.Errorf("notifications[%d]: %w", i, err)
				}
			}
			username := strings.TrimSpace(expandEnvPlaceholders(raw.SMTPUsername))
			if username != "" && password == "" {
				return nil, fmt.Errorf("notifications[%d]: email smtp_password is required when smtp_username is set", i)
			}

			channels = append(channels, channel{
				id:   fmt.Sprintf("email[%d]", i),
				kind: channelEmail,
				on:   on,
				email: &emailChannel{
					host:     host,
					port:     port,
					username: username,
					password: password,
					from:     from,
					to:       to,
					subject:  strings.TrimSpace(expandEnvPlaceholders(raw.Subject)),
				},
			})
		default:
			return nil, fmt.Errorf("notifications[%d]: unsupported type %q", i, raw.Type)
		}
	}
	return channels, nil
}

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func expandEnvPlaceholders(value string) string {
	if value == "" {
		return value
	}
	return envVarPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := envVarPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return ""
		}
		return os.Getenv(parts[1])
	})
}

func resolveSecretFromEnv(rawValue, field string) (string, error) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}

	envVars := referencedEnvVars(value)
	if len(envVars) == 0 {
		return "", fmt.Errorf("%s must use ${ENV_VAR} placeholder", field)
	}
	for _, key := range envVars {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return "", fmt.Errorf("%s references unset env var %q", field, key)
		}
	}

	resolved := strings.TrimSpace(expandEnvPlaceholders(value))
	if resolved == "" {
		return "", fmt.Errorf("%s resolved to an empty value", field)
	}
	return resolved, nil
}

func referencedEnvVars(value string) []string {
	matches := envVarPattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	vars := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		key := match[1]
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		vars = append(vars, key)
	}
	sort.Strings(vars)
	return vars
}

func isSensitiveHeader(name string) bool {
	header := strings.ToLower(strings.TrimSpace(name))
	if header == "" {
		return false
	}

	for _, marker := range []string{
		"authorization",
		"token",
		"secret",
		"password",
		"apikey",
		"api-key",
	} {
		if strings.Contains(header, marker) {
			return true
		}
	}
	return false
}

func parseEventFilters(raw []string) (map[EventType]bool, error) {
	if len(raw) == 0 {
		all := make(map[EventType]bool, len(allEventTypes))
		for _, eventType := range allEventTypes {
			all[eventType] = true
		}
		return all, nil
	}

	result := make(map[EventType]bool, len(raw))
	for _, item := range raw {
		event := EventType(strings.ToLower(strings.TrimSpace(item)))
		switch event {
		case EventNewHigh, EventNewMedium, EventNewLow, EventResolved:
			result[event] = true
		default:
			return nil, fmt.Errorf("unsupported event filter %q", item)
		}
	}
	return result, nil
}

func rateLimitKey(channelID string, finding *analyzer.Finding) string {
	location := finding.Database + "." + finding.Collection
	if finding.Index != "" {
		location += "." + finding.Index
	}
	return channelID + "|" + string(finding.Type) + "|" + location
}

func buildWebhookPayload(event *Event) ([]byte, error) {
	payload := map[string]interface{}{
		"source":    "mongospectre",
		"event":     event.Type,
		"timestamp": event.Timestamp,
		"status":    event.Status,
		"finding": map[string]string{
			"type":       string(event.Finding.Type),
			"severity":   string(event.Finding.Severity),
			"database":   event.Finding.Database,
			"collection": event.Finding.Collection,
			"index":      event.Finding.Index,
			"message":    event.Finding.Message,
		},
	}
	return json.Marshal(payload)
}

func buildSlackPayload(event *Event, dashboardURL string) ([]byte, error) {
	color := "#2eb886"
	if event.Type == EventResolved {
		color = "#439fe0"
	}
	switch event.Finding.Severity {
	case analyzer.SeverityHigh:
		color = "#d40e0d"
	case analyzer.SeverityMedium:
		color = "#f2c744"
	case analyzer.SeverityLow:
		color = "#36a64f"
	}

	location := event.Finding.Database + "." + event.Finding.Collection
	if event.Finding.Index != "" {
		location += "." + event.Finding.Index
	}

	text := fmt.Sprintf("mongospectre %s: %s (%s)", strings.ToUpper(string(event.Type)), event.Finding.Type, location)
	if dashboardURL != "" {
		text += fmt.Sprintf(" | <%s|Open dashboard>", dashboardURL)
	}

	payload := map[string]interface{}{
		"text": text,
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"fields": []map[string]interface{}{
					{"title": "Severity", "value": strings.ToUpper(string(event.Finding.Severity)), "short": true},
					{"title": "Type", "value": string(event.Finding.Type), "short": true},
					{"title": "Location", "value": location, "short": false},
					{"title": "Message", "value": event.Finding.Message, "short": false},
				},
				"footer": "mongospectre watch",
				"ts":     time.Now().Unix(),
			},
		},
	}
	return json.Marshal(payload)
}

func buildEmailMessage(event *Event, cfg *emailChannel) (string, []byte) {
	location := event.Finding.Database + "." + event.Finding.Collection
	if event.Finding.Index != "" {
		location += "." + event.Finding.Index
	}

	subject := cfg.subject
	if subject == "" {
		subject = fmt.Sprintf("[mongospectre] %s %s", strings.ToUpper(string(event.Type)), location)
	}

	htmlBody := fmt.Sprintf(`<html><body><h3>mongospectre alert: %s</h3><p><strong>Type:</strong> %s</p><p><strong>Severity:</strong> %s</p><p><strong>Location:</strong> %s</p><p><strong>Message:</strong> %s</p><p><strong>Timestamp:</strong> %s</p></body></html>`,
		event.Type,
		event.Finding.Type,
		event.Finding.Severity,
		location,
		event.Finding.Message,
		event.Timestamp,
	)

	msg := "MIME-version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		fmt.Sprintf("Subject: %s\r\n", subject) +
		fmt.Sprintf("To: %s\r\n", strings.Join(cfg.to, ",")) +
		"\r\n" +
		htmlBody

	return subject, []byte(msg)
}

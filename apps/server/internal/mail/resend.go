// Package mail sends transactional email (invites and password resets) via Resend's HTTP API.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const defaultEndpoint = "https://api.resend.com/emails"

// ErrUnconfigured is returned by Send when no API key or sender address is set.
var ErrUnconfigured = errors.New("email is not configured")

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Resend sends email through Resend. It reads its settings on every send, so a change in
// the admin console applies at once.
type Resend struct {
	endpoint string
	setting  func(key string) string
	client   HTTPClient
}

// NewResend reads resend.apiKey, resend.fromEmail, and resend.fromName through setting.
func NewResend(setting func(key string) string) *Resend {
	return &Resend{endpoint: defaultEndpoint, setting: setting, client: &http.Client{Timeout: 15 * time.Second}}
}

// Configured reports whether an API key and sender address are set.
func (r *Resend) Configured() bool {
	return r.setting("resend.apiKey") != "" && r.setting("resend.fromEmail") != ""
}

// Send delivers an HTML email to a single recipient.
func (r *Resend) Send(ctx context.Context, toEmail, toName, subject, html string) error {
	if !r.Configured() {
		return ErrUnconfigured
	}
	from := (&mail.Address{Name: strings.TrimSpace(r.setting("resend.fromName")), Address: r.setting("resend.fromEmail")}).String()
	to := (&mail.Address{Name: toName, Address: toEmail}).String()
	body, err := json.Marshal(map[string]any{"from": from, "to": []string{to}, "subject": subject, "html": html})
	if err != nil {
		return fmt.Errorf("encode resend request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create resend request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+r.setting("resend.apiKey"))

	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("resend returned %d: %s", response.StatusCode, string(responseBody))
	}
	return nil
}

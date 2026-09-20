// Package mail sends transactional email (currently just invite emails) via ZeptoMail's HTTP API.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultEndpoint = "https://api.zeptomail.com/v1.1/email"

// ErrUnconfigured is returned by Send when no API token was provided at construction.
var ErrUnconfigured = errors.New("zeptomail is not configured")

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// ZeptoMail sends email through Zoho's ZeptoMail send API.
type ZeptoMail struct {
	endpoint  string
	token     string
	fromEmail string
	fromName  string
	client    HTTPClient
}

func NewZeptoMail(token, fromEmail, fromName string) *ZeptoMail {
	return &ZeptoMail{
		endpoint:  defaultEndpoint,
		token:     strings.TrimSpace(token),
		fromEmail: strings.TrimSpace(fromEmail),
		fromName:  strings.TrimSpace(fromName),
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Configured reports whether an API token and sender address were provided.
func (z *ZeptoMail) Configured() bool { return z.token != "" && z.fromEmail != "" }

// Send delivers an HTML email to a single recipient.
func (z *ZeptoMail) Send(ctx context.Context, toEmail, toName, subject, html string) error {
	if !z.Configured() {
		return ErrUnconfigured
	}
	payload := map[string]any{
		"from": map[string]string{"address": z.fromEmail, "name": z.fromName},
		"to": []map[string]any{
			{"email_address": map[string]string{"address": toEmail, "name": toName}},
		},
		"subject":  subject,
		"htmlbody": html,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode zeptomail request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, z.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create zeptomail request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Zoho-enczapikey "+z.token)

	response, err := z.client.Do(request)
	if err != nil {
		return fmt.Errorf("send invite email: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("zeptomail returned %d: %s", response.StatusCode, string(responseBody))
	}
	return nil
}

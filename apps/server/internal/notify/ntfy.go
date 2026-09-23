// Package notify pushes administrator alerts to an ntfy topic (https://ntfy.sh or a
// self-hosted ntfy server).
package notify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrUnconfigured = errors.New("ntfy is not configured")

// Ntfy reads ntfy.url, ntfy.topic, and ntfy.token through setting on every send.
type Ntfy struct {
	setting func(key string) string
	client  *http.Client
}

func NewNtfy(setting func(key string) string) *Ntfy {
	return &Ntfy{setting: setting, client: &http.Client{Timeout: 10 * time.Second}}
}

func (n *Ntfy) Configured() bool { return n.setting("ntfy.topic") != "" }

// Send publishes message under title; tags are ntfy tags or emoji shortcodes.
func (n *Ntfy) Send(ctx context.Context, title, message string, tags ...string) error {
	if !n.Configured() {
		return ErrUnconfigured
	}
	server := strings.TrimRight(n.setting("ntfy.url"), "/")
	if server == "" {
		server = "https://ntfy.sh"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/"+n.setting("ntfy.topic"), strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("create ntfy request: %w", err)
	}
	request.Header.Set("Title", title)
	if len(tags) > 0 {
		request.Header.Set("Tags", strings.Join(tags, ","))
	}
	if token := n.setting("ntfy.token"); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := n.client.Do(request)
	if err != nil {
		return fmt.Errorf("send ntfy notification: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("ntfy returned %d: %s", response.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}

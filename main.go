package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	api_v1 "github.com/prometheus/prometheus/web/api/v1"
)

type CLI struct {
	CheckUUID           string        `short:"u" required:""`
	HealthchecksBaseURL *url.URL      `short:"b" default:"https://hc-ping.com"`
	PrometheusBaseURL   *url.URL      `short:"p" required:""`
	Timeout             time.Duration `short:"t" default:"30s"`
	Interval            time.Duration `short:"i" default:"5m"`
}

// ping sends some sort of ping message to the healthchecks server.
func (cli CLI) ping(ctx context.Context, name string, method string, url *url.URL, body io.Reader) {
	ctx, cancel := context.WithTimeout(ctx, cli.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url.String(), body)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to create ping %s request", name), "error", err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to execute ping %s request", name), "error", err)
		return
	}

	if resp.StatusCode/100 != 2 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to read ping %s response body", name), "error", err)
			body = []byte("<read-error>")
		}
		if err := resp.Body.Close(); err != nil {
			slog.Warn(fmt.Sprintf("failed to close ping %s response body", name), "error", err)
		}

		msg := fmt.Sprintf("unexpected response status code for ping %s response", name)
		slog.Error(msg, "status", resp.Status, "body", string(body))
		return
	}

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		slog.Warn(fmt.Sprintf("failed to read ping %s response body to EOF", name), "error", err)
	}
	if err := resp.Body.Close(); err != nil {
		slog.Warn(fmt.Sprintf("failed to close ping %s response body", name), "error", err)
	}
}

// ping sends a success message to the healthchecks server.
func (cli CLI) pingSuccess(ctx context.Context) {
	slog.Info("success")

	url := cli.HealthchecksBaseURL.JoinPath(cli.CheckUUID)
	cli.ping(ctx, "success", http.MethodGet, url, nil)
}

// failureMsg creates message by formatting msg and args.
func failureMsg(msg string, args ...any) string {
	var sb strings.Builder
	sb.WriteString(msg)

	rec := &slog.Record{}
	rec.Add(args...)
	rec.Attrs(func(a slog.Attr) bool {
		sb.WriteRune(' ')
		sb.WriteString(a.String())
		return true
	})

	return sb.String()
}

// ping sends a failure message containing the given msg and args to the
// healthchecks server .
func (cli CLI) pingFailure(ctx context.Context, msg string, args ...any) {
	slog.Error(msg, args...)

	url := cli.HealthchecksBaseURL.JoinPath(cli.CheckUUID, "fail")
	body := strings.NewReader(failureMsg(msg, args...))
	cli.ping(ctx, "failure", http.MethodPost, url, body)
}

// check attempts to contact prometheus, and informs healthchecks according to
// the result.
func (cli CLI) check() {
	parent, cancel := context.WithTimeout(context.Background(), cli.Interval)
	defer cancel()

	ctx, cancel := context.WithTimeout(parent, cli.Timeout)
	defer cancel()

	url := cli.PrometheusBaseURL.JoinPath("api/v1/alerts").String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cli.pingFailure(parent, "failed to create prometheus request", "error", err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cli.pingFailure(parent, "failed to execute prometheus request", "error", err)
		return
	}

	if resp.StatusCode/100 != 2 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Warn("failed to read prometheus response body", "error", err)
			body = []byte("<read-error>")
		}
		if err := resp.Body.Close(); err != nil {
			slog.Warn("failed to close prometheus response body", "error", err)
		}

		msg := "unexpected response status code for prometheus response"
		cli.pingFailure(parent, msg, "status", resp.Status, "body", string(body))
		return
	}

	var resp1 struct {
		Data api_v1.AlertDiscovery `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resp1); err != nil {
		cli.pingFailure(parent, "failed to decode prometheus response body", "error", err)
		return
	}
	if err := resp.Body.Close(); err != nil {
		slog.Warn("failed to close prometheus response body", "error", err)
	}

	if len(resp1.Data.Alerts) != 0 {
		alerts, err := json.Marshal(resp1.Data.Alerts)
		if err != nil {
			slog.Warn("failed to marshal prometheus alerts", "error", err)
			alerts = []byte("<marshal-error>")
		}

		cli.pingFailure(parent, "prometheus reported active alerts", "alerts", string(alerts))
		return
	}

	cli.pingSuccess(parent)
}

func main() {
	var cli CLI
	kong.Parse(&cli)

	cli.check()
	for range time.Tick(cli.Interval) {
		cli.check()
	}
}

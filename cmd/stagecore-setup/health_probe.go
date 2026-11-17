package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deployment"
)

func runHealthProbe(args []string) error {
	fs := flag.NewFlagSet("stagecore-setup health probe", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	endpoint := fs.String("url", "", "loopback StageCore health endpoint")
	timeout := fs.Duration("timeout", 2*time.Second, "health request timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("health probe does not accept positional arguments")
	}
	if *timeout < time.Second || *timeout > 2*time.Minute {
		return fmt.Errorf("health probe timeout must be between 1s and 2m")
	}
	result, err := performHealthProbe(context.Background(), *endpoint, *timeout, nil)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return fmt.Errorf("write health probe result: %w", err)
	}
	if result.HTTPStatus < http.StatusOK || result.HTTPStatus >= http.StatusMultipleChoices {
		return fmt.Errorf("health endpoint returned HTTP %d", result.HTTPStatus)
	}
	return nil
}

func performHealthProbe(ctx context.Context, endpoint string, timeout time.Duration, client *http.Client) (deployment.SystemdHealthProbeResult, error) {
	if err := deployment.ValidateSystemdHealthProbeURL(endpoint); err != nil {
		return deployment.SystemdHealthProbeResult{}, err
	}
	if timeout < time.Second || timeout > 2*time.Minute {
		return deployment.SystemdHealthProbeResult{}, fmt.Errorf("health probe timeout must be between 1s and 2m")
	}
	parsed, _ := url.Parse(endpoint)
	expectedStatus := strings.ToUpper(strings.TrimPrefix(parsed.Path, "/health/"))
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return deployment.SystemdHealthProbeResult{}, fmt.Errorf("create health request: %w", err)
	}
	if client == nil {
		client = &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(request)
	if err != nil {
		return deployment.SystemdHealthProbeResult{}, fmt.Errorf("request StageCore health endpoint: %w", err)
	}
	defer response.Body.Close()
	var payload struct {
		Status       string `json:"status"`
		StorageState string `json:"storage_state"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload); err != nil {
		return deployment.SystemdHealthProbeResult{}, fmt.Errorf("decode StageCore health response: %w", err)
	}
	if expectedStatus == "LIVE" && payload.Status != "LIVE" || expectedStatus == "READY" && payload.Status != "READY" {
		return deployment.SystemdHealthProbeResult{}, errors.New("health response state does not match the requested endpoint")
	}
	return deployment.SystemdHealthProbeResult{
		HTTPStatus: response.StatusCode, Status: payload.Status, StorageState: payload.StorageState,
	}, nil
}

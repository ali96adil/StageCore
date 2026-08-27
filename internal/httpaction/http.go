package httpaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

const CapabilityKey = "http.request"

const (
	defaultTimeout = 5 * time.Second
	maxTimeout     = 30 * time.Second
)

type Executor struct {
	client *http.Client
}

type targetConfig struct {
	URL       string `json:"url"`
	SecretRef string `json:"secret_ref,omitempty"`
}

type parameters struct {
	Method    string            `json:"method"`
	Path      string            `json:"path,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	SecretRef string            `json:"secret_ref,omitempty"`
}

func New() *Executor {
	return &Executor{client: &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}}
}

func NewWithClient(client *http.Client) *Executor {
	if client == nil {
		return New()
	}
	return &Executor{client: client}
}

func (e *Executor) Execute(ctx context.Context, req capability.Request) capability.Result {
	if e == nil || e.client == nil || req.Target == nil {
		return failure("HTTP_TARGET_REQUIRED", "HTTP Action requires a configured target")
	}
	var target targetConfig
	if err := json.Unmarshal(req.Target.Configuration, &target); err != nil {
		return failure("HTTP_TARGET_INVALID", "HTTP target configuration is invalid")
	}
	if strings.TrimSpace(target.SecretRef) != "" {
		return failure("SECRET_STORE_REQUIRED", "HTTP credentials require the StageCore Secret Store")
	}
	base, err := url.Parse(strings.TrimSpace(target.URL))
	if err != nil || base.Scheme == "" || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil {
		return failure("HTTP_TARGET_INVALID", "HTTP target must be an absolute http/https URL without embedded credentials")
	}
	if hasSensitiveQuery(base.Query()) {
		return failure("SECRET_STORE_REQUIRED", "credential-like HTTP query parameters require the StageCore Secret Store")
	}

	var p parameters
	if len(req.Parameters) == 0 {
		req.Parameters = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(req.Parameters, &p); err != nil {
		return failure("HTTP_INVALID_PARAMETERS", "HTTP Action parameters are invalid")
	}
	if strings.TrimSpace(p.SecretRef) != "" {
		return failure("SECRET_STORE_REQUIRED", "HTTP credentials require the StageCore Secret Store")
	}
	method := strings.ToUpper(strings.TrimSpace(p.Method))
	if method == "" {
		method = http.MethodPost
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return failure("HTTP_METHOD_UNSUPPORTED", "HTTP method is not supported by the basic MVP adapter")
	}
	for name := range p.Headers {
		if sensitiveHeader(name) {
			return failure("SECRET_STORE_REQUIRED", "credential-bearing HTTP headers require the StageCore Secret Store")
		}
	}

	targetURL := *base
	if path := strings.TrimSpace(p.Path); path != "" {
		relative, err := url.Parse(path)
		if err != nil || relative.IsAbs() || relative.Host != "" || relative.User != nil || hasSensitiveQuery(relative.Query()) {
			return failure("HTTP_INVALID_PARAMETERS", "HTTP relative path is invalid or contains credential-like query parameters")
		}
		targetURL = *base.ResolveReference(relative)
	}

	timeout := boundedTimeout(req.TimeoutMS)
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(execCtx, method, targetURL.String(), bytes.NewBufferString(p.Body))
	if err != nil {
		return failure("HTTP_REQUEST_INVALID", "HTTP request could not be constructed")
	}
	for name, value := range p.Headers {
		httpReq.Header.Set(name, value)
	}

	response, err := e.client.Do(httpReq)
	if err != nil {
		switch {
		case errors.Is(execCtx.Err(), context.DeadlineExceeded):
			return capability.Result{Result: domain.ExecutionTimedOut, AckLevel: contracts.AckNone, ErrorCode: "HTTP_TIMEOUT", ResponseSummary: "HTTP Action exceeded its bounded timeout"}
		case errors.Is(execCtx.Err(), context.Canceled):
			return capability.Result{Result: domain.ExecutionCancelled, AckLevel: contracts.AckNone, ErrorCode: "HTTP_CANCELLED", ResponseSummary: "HTTP Action was cancelled"}
		default:
			return failure("HTTP_TRANSPORT_FAILED", "HTTP request failed before a response was received")
		}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	summary := fmt.Sprintf("HTTP %d %s", response.StatusCode, http.StatusText(response.StatusCode))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckAccepted, ResponseSummary: summary}
	}
	return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckAccepted, ErrorCode: fmt.Sprintf("HTTP_STATUS_%d", response.StatusCode), ResponseSummary: summary}
}

func boundedTimeout(timeoutMS int64) time.Duration {
	if timeoutMS <= 0 {
		return defaultTimeout
	}
	value := time.Duration(timeoutMS) * time.Millisecond
	if value > maxTimeout {
		return maxTimeout
	}
	return value
}

func sensitiveHeader(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "authorization" || name == "proxy-authorization" || name == "cookie" || name == "set-cookie" {
		return true
	}
	return strings.Contains(name, "token") || strings.Contains(name, "secret") || strings.Contains(name, "api-key") || strings.Contains(name, "apikey")
}

func hasSensitiveQuery(values url.Values) bool {
	for key := range values {
		key = strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") || strings.Contains(key, "api_key") || strings.Contains(key, "apikey") || strings.Contains(key, "key") {
			return true
		}
	}
	return false
}

func failure(code, summary string) capability.Result {
	return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: code, ResponseSummary: summary}
}

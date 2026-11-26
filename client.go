package fluxsend

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
)

const (
	defaultUserAgent  = "fluxsend-go-sdk/0.1"
	defaultMaxRetries = 2
	maxErrorBodyBytes = 1 << 20
	maxJSONBodyBytes  = 16 << 20
	retryBaseDelay    = 100 * time.Millisecond
	retryMaxDelay     = 2 * time.Second
)

// Config controls a FluxSend client.
type Config struct {
	BaseURL           string
	APIKey            string
	HTTPClient        *http.Client
	UserAgent         string
	AllowInsecureHTTP bool
}

// Client executes FluxSend developer API operations.
type Client struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

type request struct {
	method        string
	path          string
	query         url.Values
	body          any
	contentType   string
	contentLength int64
	requiresAuth  bool
	retryable     bool
}

type response struct {
	statusCode int
	header     http.Header
	data       json.RawMessage
}

// Download owns a streaming HTTP response body.
type Download struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

type envelope struct {
	Status   int             `json:"status"`
	Msg      string          `json:"msg,omitempty"`
	Response json.RawMessage `json:"response"`
}

// NewClient validates configuration and returns a concurrency-safe client.
func NewClient(config Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("base URL must use http or https")
	}
	if baseURL.Scheme == "http" && !config.AllowInsecureHTTP {
		return nil, fmt.Errorf("http base URL requires AllowInsecureHTTP")
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("base URL must include a host")
	}
	if baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("base URL must not contain a query or fragment")
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	return &Client{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(config.APIKey),
		httpClient: httpClient,
		userAgent:  userAgent,
	}, nil
}

func (c *Client) doJSON[T any](ctx context.Context, request request) (T, error) {
	var zero T
	if request.body != nil && request.contentType == "" {
		request.contentType = "application/json"
	}
	result, err := c.do(ctx, request)
	if err != nil {
		return zero, err
	}
	if len(result.data) == 0 || bytes.Equal(result.data, []byte("null")) {
		return zero, nil
	}
	var value T
	if err := json.Unmarshal(result.data, &value); err != nil {
		return zero, fmt.Errorf("%w: decode response data: %v", ErrAPI, err)
	}
	return value, nil
}

func (c *Client) do(ctx context.Context, request request) (*response, error) {
	body, err := encodeJSONBody(request.body)
	if err != nil {
		return nil, err
	}

	attempts := 1
	if request.retryable {
		attempts += defaultMaxRetries
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		result, retryAfter, err := c.execute(ctx, request, body)
		if err == nil {
			return result, nil
		}
		if attempt == attempts || !shouldRetry(err, retryAfter) {
			return nil, err
		}
		if err := waitForRetry(ctx, attempt, retryAfter); err != nil {
			return nil, err
		}
	}
	panic("unreachable")
}

func (c *Client) execute(ctx context.Context, request request, body []byte) (*response, time.Duration, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := c.newHTTPRequest(ctx, request, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	resp, err := c.httpClientForRequest().Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, 0, err
		}
		return nil, 0, fmt.Errorf("%w: %v", ErrTransport, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		apiErr := readAPIError(resp)
		return nil, apiErr.RetryAfter, apiErr
	}
	result, err := readJSONResponse(resp)
	return result, 0, err
}

func (c *Client) doReaderJSON[T any](ctx context.Context, request request, body io.Reader) (T, error) {
	var zero T
	req, err := c.newHTTPRequest(ctx, request, body)
	if err != nil {
		return zero, err
	}
	resp, err := c.httpClientForRequest().Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return zero, err
		}
		return zero, fmt.Errorf("%w: %v", ErrTransport, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return zero, readAPIError(resp)
	}
	result, err := readJSONResponse(resp)
	if err != nil {
		return zero, err
	}
	if len(result.data) == 0 || bytes.Equal(result.data, []byte("null")) {
		return zero, nil
	}
	var value T
	if err := json.Unmarshal(result.data, &value); err != nil {
		return zero, fmt.Errorf("%w: decode response data: %v", ErrAPI, err)
	}
	return value, nil
}

func (c *Client) doStream(ctx context.Context, request request) (*Download, error) {
	attempts := 1
	if request.retryable {
		attempts += defaultMaxRetries
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		result, retryAfter, err := c.executeStream(ctx, request)
		if err == nil {
			return result, nil
		}
		if attempt == attempts || !shouldRetry(err, retryAfter) {
			return nil, err
		}
		if err := waitForRetry(ctx, attempt, retryAfter); err != nil {
			return nil, err
		}
	}
	panic("unreachable")
}

func (c *Client) executeStream(ctx context.Context, request request) (*Download, time.Duration, error) {
	req, err := c.newHTTPRequest(ctx, request, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.httpClientForRequest().Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, 0, err
		}
		return nil, 0, fmt.Errorf("%w: %v", ErrTransport, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()
		apiErr := readAPIError(resp)
		return nil, apiErr.RetryAfter, apiErr
	}
	return &Download{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       resp.Body,
	}, 0, nil
}

func (c *Client) newHTTPRequest(ctx context.Context, request request, body io.Reader) (*http.Request, error) {
	if request.requiresAuth && c.apiKey == "" {
		return nil, fmt.Errorf("%w: API key is required", ErrAuthentication)
	}
	endpoint, err := url.Parse(c.baseURL.String() + request.path)
	if err != nil {
		return nil, fmt.Errorf("build endpoint URL: %w", err)
	}
	endpoint.RawQuery = request.query.Encode()

	req, err := http.NewRequestWithContext(ctx, request.method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if request.contentType != "" {
		req.Header.Set("Content-Type", request.contentType)
	}
	if request.requiresAuth && c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	if request.contentLength > 0 {
		req.ContentLength = request.contentLength
	}
	return req, nil
}

func (c *Client) httpClientForRequest() *http.Client {
	client := *c.httpClient
	previousCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, previous []*http.Request) error {
		if next.URL.Host != c.baseURL.Host {
			next.Header.Del("X-API-Key")
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(next, previous)
		}
		if len(previous) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}

func readJSONResponse(resp *http.Response) (*response, error) {
	rawBody, err := readBounded(resp.Body, maxJSONBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrTransport, err)
	}
	var responseEnvelope envelope
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &responseEnvelope); err != nil {
			return nil, fmt.Errorf("%w: decode response envelope: %v", ErrAPI, err)
		}
		if responseEnvelope.Response == nil {
			return nil, fmt.Errorf("%w: response envelope has no response field", ErrAPI)
		}
	}
	return &response{
		statusCode: resp.StatusCode,
		header:     resp.Header.Clone(),
		data:       responseEnvelope.Response,
	}, nil
}

func readBounded(body io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return raw, nil
}

func readAPIError(resp *http.Response) *APIError {
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	var responseEnvelope envelope
	if len(rawBody) > 0 {
		_ = json.Unmarshal(rawBody, &responseEnvelope)
	}
	return apiError(resp.StatusCode, responseEnvelope.Msg, responseEnvelope.Response, resp.Header)
}

func encodeJSONBody(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode JSON body: %w", err)
	}
	return encoded, nil
}

func shouldRetry(err error, retryAfter time.Duration) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	return errors.Is(err, ErrTransport)
}

func waitForRetry(ctx context.Context, attempt int, retryAfter time.Duration) error {
	delay := retryAfter
	if delay <= 0 {
		delay = retryBaseDelay * time.Duration(1<<(attempt-1))
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

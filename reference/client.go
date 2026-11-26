// Package reference contains generated, callable examples of the FluxSend CLI
// API. It is a development reference, not the final public SDK.
package reference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

const userAgent = "fluxsend-sdk-reference/0.1"

// Client executes reference API calls against one FluxSend deployment.
type Client struct {
	BaseURL    *url.URL
	APIKey     string
	HTTPClient *http.Client
}

// Call describes one HTTP request derived from docs/swagger.json.
type Call struct {
	Method        string
	Path          string
	Query         url.Values
	Body          any
	ContentType   string
	ContentLength int64
}

// Envelope is the JSON wrapper used by JSON FluxSend responses.
type Envelope struct {
	Status   int             `json:"status"`
	Msg      string          `json:"msg,omitempty"`
	Response json.RawMessage `json:"response"`
}

// Result keeps the HTTP response available for inspection by the caller.
type Result struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Envelope   *Envelope
}

// Decode unmarshals the operation-specific response payload into T.
func Decode[T any](result *Result) (T, error) {
	var zero T
	if result == nil || result.Envelope == nil {
		return zero, fmt.Errorf("decode response: missing JSON envelope")
	}
	if len(result.Envelope.Response) == 0 || bytes.Equal(result.Envelope.Response, []byte("null")) {
		return zero, nil
	}
	var value T
	if err := json.Unmarshal(result.Envelope.Response, &value); err != nil {
		return zero, fmt.Errorf("decode response payload: %w", err)
	}
	return value, nil
}

// APIError is returned for non-2xx responses.
type APIError struct {
	StatusCode int
	Message    string
	Result     *Result
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("fluxsend: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("fluxsend: HTTP %d", e.StatusCode)
}

// NewClient validates the deployment origin and creates a client.
func NewClient(baseURL, apiKey string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base URL must include scheme and host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("base URL must not contain a query or fragment")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{BaseURL: parsed, APIKey: apiKey, HTTPClient: httpClient}, nil
}

// Do executes a call and returns the raw response plus its parsed JSON envelope.
func (c *Client) Do(ctx context.Context, call Call) (*Result, error) {
	endpoint, err := url.Parse(c.BaseURL.String() + call.Path)
	if err != nil {
		return nil, fmt.Errorf("build endpoint URL: %w", err)
	}
	endpoint.RawQuery = call.Query.Encode()

	body, contentType, err := encodeBody(call.Body, call.ContentType)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, call.Method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if call.ContentLength > 0 {
		req.ContentLength = call.ContentLength
	}
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	result := &Result{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       responseBody,
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		var envelope Envelope
		if json.Unmarshal(responseBody, &envelope) == nil && envelope.Status != 0 {
			result.Envelope = &envelope
		}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := ""
		if result.Envelope != nil {
			message = result.Envelope.Msg
		}
		return result, &APIError{StatusCode: resp.StatusCode, Message: message, Result: result}
	}
	return result, nil
}

func encodeBody(value any, contentType string) (io.Reader, string, error) {
	if value == nil {
		return nil, "", nil
	}
	switch body := value.(type) {
	case io.Reader:
		return body, contentType, nil
	case []byte:
		if contentType == "" {
			contentType = "application/json"
		}
		return bytes.NewReader(body), contentType, nil
	case string:
		if contentType == "" {
			contentType = "application/json"
		}
		return strings.NewReader(body), contentType, nil
	default:
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, "", fmt.Errorf("encode JSON body: %w", err)
		}
		if contentType == "" {
			contentType = "application/json"
		}
		return bytes.NewReader(encoded), contentType, nil
	}
}

func multipartUpload(ctx context.Context, c *Client, path string, contents io.Reader, fileName, folder string) (*Result, error) {
	// shortcut: this reference buffers multipart uploads; stream them before shipping the SDK.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("create multipart file field: %w", err)
	}
	if _, err := io.Copy(part, contents); err != nil {
		return nil, fmt.Errorf("copy multipart file: %w", err)
	}
	if folder != "" {
		if err := writer.WriteField("folder", folder); err != nil {
			return nil, fmt.Errorf("write multipart folder: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart body: %w", err)
	}

	return c.Do(ctx, Call{
		Method:      http.MethodPost,
		Path:        path,
		Body:        &body,
		ContentType: writer.FormDataContentType(),
	})
}

func escapePath(value string) string {
	return url.PathEscape(value)
}

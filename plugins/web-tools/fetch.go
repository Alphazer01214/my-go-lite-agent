package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type fetchIn struct {
	URL       string            `json:"url"`
	Method    string            `json:"method,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	MaxBytes  *int              `json:"max_bytes,omitempty"`
	TimeoutMS *int              `json:"timeout_ms,omitempty"`
}

func fetch(ctx context.Context, in fetchIn) (json.RawMessage, error) {
	return fetchWithClient(ctx, in, newSafeClient())
}

func fetchWithClient(ctx context.Context, in fetchIn, client *http.Client) (json.RawMessage, error) {
	parsed, err := validateURL(in.URL)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(in.Method)
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "method must be GET or HEAD")
	}
	maxBytes := 1 << 20
	if in.MaxBytes != nil {
		maxBytes = *in.MaxBytes
	}
	timeoutMS := 30000
	if in.TimeoutMS != nil {
		timeoutMS = *in.TimeoutMS
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, parsed.String(), nil)
	if err != nil {
		return nil, pluginsdk.ErrCode("url_invalid", err.Error())
	}
	for key, value := range in.Headers {
		if blockedHeader(key) {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "header is not allowed: "+key)
		}
		if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "header contains a newline")
		}
		request.Header.Set(key, value)
	}
	started := time.Now()
	redirects := 0
	copied := *client
	originalHost := parsed.Host
	baseCheck := copied.CheckRedirect
	copied.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errRedirectLimit
		}
		if _, err := validateURL(next.URL.String()); err != nil {
			return err
		}
		redirects = len(via)
		if next.URL.Host != originalHost {
			next.Header.Del("Authorization")
			next.Header.Del("Cookie")
			next.Header.Del("Proxy-Authorization")
		}
		if baseCheck != nil {
			return baseCheck(next, via)
		}
		return nil
	}
	response, err := copied.Do(request)
	if err != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return toolcontract.Failure("upstream_error", "request timed out", map[string]any{
				"timed_out": true, "url": parsed.String(),
			})
		}
		return nil, fetchError(err, requestCtx, ctx)
	}
	defer response.Body.Close()
	contentType := response.Header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(contentType)
	if mediaType != "" && !supportedContentType(mediaType) {
		return toolcontract.Failure("content_type_unsupported", "response content type is not text", map[string]any{
			"content_type": contentType, "status_code": response.StatusCode,
		})
	}
	if charset := strings.ToLower(strings.TrimSpace(params["charset"])); charset != "" && charset != "utf-8" && charset != "utf8" {
		return toolcontract.Failure("charset_unsupported", "only UTF-8 responses are supported", map[string]any{
			"charset": charset, "content_type": contentType,
		})
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maxBytes)+1))
	if err != nil {
		return nil, fetchError(err, requestCtx, ctx)
	}
	truncated := len(body) > maxBytes
	if truncated {
		body = body[:maxBytes]
		for !utf8.Valid(body) {
			body = body[:len(body)-1]
		}
	}
	finalURL := response.Request.URL.String()
	meta := map[string]any{
		"final_url":    finalURL,
		"status_code":  response.StatusCode,
		"content_type": contentType,
		"bytes":        len(body),
		"truncated":    truncated,
		"redirects":    redirects,
		"fetched_at":   time.Now().UTC().Format(time.RFC3339Nano),
		"method":       method,
		"duration_ms":  time.Since(started).Milliseconds(),
	}
	if !utf8.Valid(body) {
		return toolcontract.Failure("charset_unsupported", "response is not valid UTF-8", meta)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return toolcontract.Failure("upstream_error", "upstream returned HTTP "+strconv.Itoa(response.StatusCode), meta)
	}
	return toolcontract.Encode(toolcontract.Result{Content: string(body), Truncated: truncated, Meta: meta})
}

var errRedirectLimit = errors.New("redirect_limit")

func fetchError(err error, requestCtx, parent context.Context) error {
	if errors.Is(err, errRedirectLimit) {
		return pluginsdk.ErrCode("redirect_limit", "too many redirects")
	}
	if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
		if errors.Is(parent.Err(), context.DeadlineExceeded) {
			return pluginsdk.ErrCode(pluginsdk.CodeCallDeadlineExceeded, "call deadline exceeded")
		}
		return pluginsdk.ErrCode("upstream_error", "request timed out")
	}
	if errors.Is(parent.Err(), context.Canceled) {
		return pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "call cancelled")
	}
	if strings.Contains(err.Error(), "address_blocked") {
		return pluginsdk.ErrCode("address_blocked", "private or reserved address is blocked")
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return pluginsdk.ErrCode("upstream_error", urlErr.Err.Error())
	}
	return pluginsdk.ErrCode("upstream_error", err.Error())
}

func validateURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return nil, pluginsdk.ErrCode("url_invalid", "invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, pluginsdk.ErrCode("scheme_not_allowed", "only http and https are allowed")
	}
	if parsed.User != nil {
		return nil, pluginsdk.ErrCode("url_invalid", "URL user information is not allowed")
	}
	return parsed, nil
}

func supportedContentType(value string) bool {
	value = strings.ToLower(value)
	return strings.HasPrefix(value, "text/") ||
		value == "application/json" ||
		value == "application/xml" ||
		value == "application/xhtml+xml" ||
		value == "application/javascript" ||
		value == "application/yaml" ||
		value == "application/x-yaml"
}

func blockedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "connection", "content-length", "transfer-encoding":
		return true
	default:
		return false
	}
}

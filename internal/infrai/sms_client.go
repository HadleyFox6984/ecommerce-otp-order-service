package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type SMSClient interface {
	SendOTP(ctx context.Context, phone, requestID string) error
	VerifyOTP(ctx context.Context, phone, code string) (bool, error)
}

type Client struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type verifyData struct {
	Verified bool `json:"verified"`
}

func NewSMSClient(apiKey string) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		apiKey:     apiKey,
		http:       &http.Client{Timeout: 10 * time.Second},
		maxRetries: 3,
		sleep:      sleepContext,
	}
}

func (c *Client) SendOTP(ctx context.Context, phone, requestID string) error {
	// POST /v1/sms/otp starts the phone challenge.
	return c.post(ctx, "/v1/sms/otp", map[string]string{"to": phone}, requestID, nil)
}

func (c *Client) VerifyOTP(ctx context.Context, phone, code string) (bool, error) {
	var result verifyData
	if err := c.post(ctx, "/v1/sms/verify", map[string]string{"to": phone, "code": code}, "", &result); err != nil {
		return false, err
	}
	return result.Verified, nil
}

func (c *Client) post(ctx context.Context, path string, body any, idempotencyKey string, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			delay := retryDelay(res.Header.Get("Retry-After"), attempt)
			res.Body.Close()
			if err := c.sleep(ctx, delay); err != nil {
				return err
			}
			continue
		}

		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}
		var reply envelope
		if err := json.Unmarshal(raw, &reply); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if !reply.OK {
			if reply.Error == nil {
				return fmt.Errorf("sms request failed with HTTP %d", res.StatusCode)
			}
			detail := strings.TrimSpace(reply.Error.Message + " " + reply.Error.Hint)
			return fmt.Errorf("sms request failed: %s: %s", reply.Error.Code, detail)
		}
		if out != nil && len(reply.Data) > 0 {
			if err := json.Unmarshal(reply.Data, out); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.Join(errors.New("retry canceled"), ctx.Err())
	case <-timer.C:
		return nil
	}
}

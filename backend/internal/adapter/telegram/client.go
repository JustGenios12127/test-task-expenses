package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	apiBase         = "https://api.telegram.org"
	longPollTimeout = 25
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		baseURL: apiBase,
		token:   token,
		httpClient: &http.Client{
			Timeout: (longPollTimeout + 10) * time.Second,
		},
	}
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	Text string `json:"text"`
	From *User  `json:"from"`
	Chat Chat   `json:"chat"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type sendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

func (c *Client) SendText(ctx context.Context, chatID int64, text string) error {
	body := sendMessageRequest{ChatID: chatID, Text: text}
	if err := c.do(ctx, http.MethodPost, "sendMessage", nil, body, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteWebhook(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "deleteWebhook", nil, nil, nil)
}

func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	query := url.Values{}
	query.Set("timeout", strconv.Itoa(longPollTimeout))
	query.Set("allowed_updates", `["message"]`)
	if offset > 0 {
		query.Set("offset", strconv.FormatInt(offset, 10))
	}

	var updates []Update
	if err := c.do(ctx, http.MethodGet, "getUpdates", query, nil, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) do(ctx context.Context, method, apiMethod string, query url.Values, body any, dest any) error {
	endpoint := c.baseURL + "/bot" + c.token + "/" + apiMethod
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", apiMethod, err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return fmt.Errorf("build %s request: %w", apiMethod, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", apiMethod, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read %s response: %w", apiMethod, err)
	}

	var parsed apiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("decode %s response: %w", apiMethod, err)
	}
	if !parsed.OK {
		if parsed.Description == "" {
			return fmt.Errorf("telegram %s: status %d", apiMethod, resp.StatusCode)
		}
		return fmt.Errorf("telegram %s: %s", apiMethod, parsed.Description)
	}
	if dest == nil || len(parsed.Result) == 0 || string(parsed.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(parsed.Result, dest); err != nil {
		return fmt.Errorf("decode %s result: %w", apiMethod, err)
	}
	return nil
}

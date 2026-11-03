package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	tele "gopkg.in/telebot.v3"
)

const (
	longPollTimeout = 25 * time.Second
	retryDelay      = 3 * time.Second
)

type Client struct {
	api *tele.Bot
}

func NewClient(token string) (*Client, error) {
	return newClient(tele.Settings{
		Token:       token,
		Poller:      &poller{timeout: longPollTimeout, retry: retryDelay},
		Synchronous: true,
		Offline:     true,
		OnError:     logError,
		Client:      &http.Client{Timeout: longPollTimeout + 10*time.Second},
	})
}

func newClient(settings tele.Settings) (*Client, error) {
	api, err := tele.NewBot(settings)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", cleanError(err))
	}
	return &Client{api: api}, nil
}

func (c *Client) SendText(ctx context.Context, chatID int64, text string) error {
	_, err := call(ctx, func() (*tele.Message, error) {
		return c.api.Send(tele.ChatID(chatID), text)
	})
	if err != nil {
		return fmt.Errorf("telegram send message: %w", cleanError(err))
	}
	return nil
}

func (c *Client) Profile(ctx context.Context, userID int64) (string, string, error) {
	chat, err := call(ctx, func() (*tele.Chat, error) {
		return c.api.ChatByID(userID)
	})
	if err != nil {
		return "", "", fmt.Errorf("telegram get chat: %w", cleanError(err))
	}
	return chat.Username, chat.FirstName, nil
}

func (c *Client) connect() error {
	data, err := c.api.Raw("getMe", nil)
	if err != nil {
		return fmt.Errorf("telegram get me: %w", cleanError(err))
	}
	var resp struct {
		Result tele.User `json:"result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("decode telegram get me: %w", err)
	}
	c.api.Me = &resp.Result
	if err := c.api.RemoveWebhook(); err != nil {
		return fmt.Errorf("telegram delete webhook: %w", cleanError(err))
	}
	return nil
}

func call[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, err := fn()
		done <- result{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case res := <-done:
		return res.value, res.err
	}
}

func cleanError(err error) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return fmt.Errorf("%s request: %w", urlErr.Op, urlErr.Err)
	}
	return err
}

func logError(err error, _ tele.Context) {
	slog.Error("telegram", "err", cleanError(err))
}

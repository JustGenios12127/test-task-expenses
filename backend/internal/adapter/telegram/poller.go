package telegram

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	tele "gopkg.in/telebot.v3"
)

type poller struct {
	timeout time.Duration
	retry   time.Duration
}

func (p *poller) Poll(api *tele.Bot, dest chan tele.Update, stop chan struct{}) {
	offset := 0
	for {
		select {
		case <-stop:
			return
		default:
		}

		updates, err := fetchUpdates(api, offset, p.timeout)
		if err != nil {
			slog.Error("telegram get updates", "err", cleanError(err))
			select {
			case <-stop:
				return
			case <-time.After(p.retry):
			}
			continue
		}

		for _, upd := range updates {
			offset = upd.ID + 1
			select {
			case dest <- upd:
			case <-stop:
				return
			}
		}
	}
}

func fetchUpdates(api *tele.Bot, offset int, timeout time.Duration) ([]tele.Update, error) {
	data, err := api.Raw("getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         int(timeout / time.Second),
		"allowed_updates": []string{"message"},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result []tele.Update `json:"result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode updates: %w", err)
	}
	return resp.Result, nil
}

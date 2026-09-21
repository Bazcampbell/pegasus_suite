// logger/telegram/client.go

package telegram

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Bazcampbell/goreq"
)

type RetryAfterError struct{ After time.Duration }

func (e RetryAfterError) Error() string { return fmt.Sprintf("telegram: retry after %s", e.After) }

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (c *client) call(method string, body any) error {
	resp, err := goreq.Post(c.baseURL+"/bot"+c.token+"/"+method, body, &goreq.Options{Client: c.http})
	if err != nil {
		return c.redact(fmt.Errorf("telegram %s: %w", method, err))
	}

	var out apiResponse
	if err := resp.JSON(&out); err != nil {
		return fmt.Errorf("telegram %s: status %d", method, resp.StatusCode)
	}
	if !out.OK {
		if out.Parameters.RetryAfter > 0 {
			return RetryAfterError{After: time.Duration(out.Parameters.RetryAfter) * time.Second}
		}
		return fmt.Errorf("telegram %s: %s", method, out.Description)
	}
	return nil
}

func (c *client) redact(err error) error {
	if c.token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), c.token, "<token>"))
}

func (c *client) check() error {
	return c.call("getMe", struct{}{})
}

func (c *client) send(chatID int64, html string) error {
	return c.call("sendMessage", map[string]any{
		"chat_id":                  chatID,
		"text":                     html,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
}

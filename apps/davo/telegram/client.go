// davo/telegram/client.go

package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"pegasus_suite/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

const maxPhotoBytes = 15 << 20 // 15 MiB

const downloadTimeout = 30 * time.Second

type Message struct {
	Text        string
	PhotoFileID string
}

type Client struct {
	bot             *tgbotapi.BotAPI
	scrapeChannelID int64
	http            *http.Client
}

func NewClient(token string, scrapeChannelID int64) (*Client, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("creating bot: %w", err)
	}

	logger.Info(logger.Log{App: "davo", Message: "telegram connected account=" + bot.Self.UserName})

	return &Client{
		bot:             bot,
		scrapeChannelID: scrapeChannelID,
		http:            &http.Client{Timeout: downloadTimeout},
	}, nil
}

func (c *Client) Poll(ctx context.Context, onScrape func(ctx context.Context, msg Message)) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := c.bot.GetUpdatesChan(u)
	if err != nil {
		return fmt.Errorf("getting updates channel: %w", err)
	}

	startTime := time.Now().Unix()

	for {
		select {
		case update := <-updates:
			// skip messages sent pre initialisation
			if update.ChannelPost != nil && int64(update.ChannelPost.Date) < startTime {
				continue
			}

			if update.ChannelPost != nil && update.ChannelPost.Chat.ID == c.scrapeChannelID {
				onScrape(ctx, toMessage(update.ChannelPost))
			}

		case <-ctx.Done():
			c.bot.StopReceivingUpdates()
			return ctx.Err()
		}
	}
}

func toMessage(post *tgbotapi.Message) Message {
	msg := Message{Text: post.Text}

	// A post carrying media puts its text in Caption, leaving Text empty.
	if msg.Text == "" {
		msg.Text = post.Caption
	}

	// Telegram sends the same photo at several sizes, ascending. Take the largest
	// - a downscaled thumbnail is exactly what we cannot read text off.
	if post.Photo != nil {
		if sizes := *post.Photo; len(sizes) > 0 {
			largest := sizes[len(sizes)-1]
			msg.PhotoFileID = largest.FileID
		}
	}

	return msg
}

// DownloadFile fetches a file's bytes from Telegram. The direct URL embeds the
// bot token, so it is resolved and consumed here and never handed to a caller.
func (c *Client) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	url, err := c.bot.GetFileDirectURL(fileID)
	if err != nil {
		return nil, fmt.Errorf("resolving file URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building file request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading file: telegram returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	if len(body) > maxPhotoBytes {
		return nil, fmt.Errorf("file exceeds %d byte limit", maxPhotoBytes)
	}

	return body, nil
}

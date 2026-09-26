// davo/anthropic/client.go

package anthropic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"pegasus_suite/logger"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	ocrModel = "claude-opus-5"

	ocrMaxTokens = 1024

	ocrEffort = anthropic.OutputConfigEffortLow

	// Sentinel the model returns when there is nothing to read, so an image with
	// no tip in it is distinguishable from a failed call.
	noTextSentinel = "NO_TEXT"
)

const systemPrompt = `You transcribe screenshots of horse racing tip messages.

Reply with the text content of the image as plain text, preserving line breaks.

Rules:
- Transcribe exactly what is written. Never correct spelling, even where a runner name looks misspelled. The misspelling is meaningful and is handled downstream.
- Reproduce "#", "$", "R", digits and decimal points exactly as they appear.
- Include every line of the tip: race number, runner number, runner name, stake or units, market, and rated odds.
- Do not add commentary, headings, labels, or markdown code fences.
- If the image contains no readable tip text, reply with exactly: ` + noTextSentinel

var supportedMediaTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

type Client struct {
	api anthropic.Client
}

func NewClient(apiKey string) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("anthropic api key not set")
	}

	return &Client{
		api: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}, nil
}

func (c *Client) ExtractText(ctx context.Context, image []byte) (string, error) {
	if len(image) == 0 {
		return "", fmt.Errorf("empty image")
	}

	mediaType := http.DetectContentType(image)
	if !supportedMediaTypes[mediaType] {
		return "", fmt.Errorf("unsupported image type %q", mediaType)
	}

	logger.Debug(logger.Log{
		Message: fmt.Sprintf("anthropic ocr: request model=%v media_type=%v bytes=%v", ocrModel, mediaType, len(image)),
	})

	msg, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     ocrModel,
		MaxTokens: ocrMaxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: ocrEffort,
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64(mediaType, base64.StdEncoding.EncodeToString(image)),
				anthropic.NewTextBlock("Transcribe the tip in this image."),
			),
		},
	})
	if err != nil {
		logAPIError("ocr", err)
		return "", fmt.Errorf("anthropic request: %w", err)
	}

	if msg.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("request refused: %s", msg.StopDetails.Explanation)
	}

	logger.Debug(logger.Log{
		Message: fmt.Sprintf("anthropic ocr: usage input_tokens=%v output_tokens=%v stop_reason=%v", msg.Usage.InputTokens, msg.Usage.OutputTokens, msg.StopReason),
	})

	out := responseText(msg)
	if out == "" {
		return "", fmt.Errorf("no text returned")
	}
	if out == noTextSentinel {
		return "", fmt.Errorf("no tip text found in image")
	}

	return out, nil
}

// logAPIError raises a non-2xx from the API to ERROR level. These are worth
// shouting about: a 401 means every escalation is dead until the key is fixed,
// and a 429 means bets are being dropped. The request ID is included so it can
// be quoted to Anthropic support.
func logAPIError(op string, err error) {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		// Transport-level failure - no HTTP response came back at all.
		logger.Error(logger.Log{
			Message: fmt.Sprintf("anthropic request failed op=%v error=%v", op, err),
		})
		return
	}

	logger.Error(logger.Log{
		Message: fmt.Sprintf("anthropic returned an error response op=%v status=%v type=%v request_id=%v error=%v", op, apiErr.StatusCode, apiErr.Type(), apiErr.RequestID, err),
	})
}

func responseText(msg *anthropic.Message) string {
	var text strings.Builder
	for _, block := range msg.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	return strings.TrimSpace(text.String())
}

// exchange/stream.go
//
// The Exchange Stream API: one TLS connection carrying CRLF-delimited JSON.

package exchange

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const (
	streamAddr   = "stream-api.betfair.com:443"
	dialTimeout  = 10 * time.Second
	heartbeatMs  = 5000
	readDeadline = 3 * heartbeatMs * time.Millisecond
)

type Stream struct {
	conn   *tls.Conn
	reader *bufio.Reader
	nextID int
}

// StreamMessage is any message the stream sends: connection, status or mcm.
type StreamMessage struct {
	Op string `json:"op"`

	StatusCode       string `json:"statusCode"`
	ErrorCode        string `json:"errorCode"`
	ErrorMessage     string `json:"errorMessage"`
	ConnectionClosed bool   `json:"connectionClosed"`

	Clk        string         `json:"clk"`
	InitialClk string         `json:"initialClk"`
	MC         []MarketChange `json:"mc"`
}

type MarketChange struct {
	ID               string                  `json:"id"`
	Img              bool                    `json:"img"`
	MarketDefinition *StreamMarketDefinition `json:"marketDefinition"`
	RC               []RunnerChange          `json:"rc"`
}

type StreamMarketDefinition struct {
	Status MarketStatus `json:"status"`
}

// RunnerChange carries only what changed. Each ladder entry is [level, price, size];
// a zero size clears the level. A zero LTP means unchanged.
type RunnerChange struct {
	ID   int64        `json:"id"`
	LTP  float64      `json:"ltp"`
	BATB [][3]float64 `json:"batb"`
	BATL [][3]float64 `json:"batl"`
}

type streamRequest struct {
	Op               string            `json:"op"`
	ID               int               `json:"id"`
	AppKey           string            `json:"appKey,omitempty"`
	Session          string            `json:"session,omitempty"`
	MarketFilter     *streamFilter     `json:"marketFilter,omitempty"`
	MarketDataFilter *marketDataFilter `json:"marketDataFilter,omitempty"`
	HeartbeatMs      int               `json:"heartbeatMs,omitempty"`
	ConflateMs       int               `json:"conflateMs"`
	Clk              string            `json:"clk,omitempty"`
	InitialClk       string            `json:"initialClk,omitempty"`
}

type streamFilter struct {
	MarketIDs []string `json:"marketIds"`
}

type marketDataFilter struct {
	Fields       []string `json:"fields"`
	LadderLevels int      `json:"ladderLevels"`
}

// DialStream connects and authenticates a stream with the client's app key and session.
func (c *Client) DialStream() (*Stream, error) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, "tcp", streamAddr, nil)
	if err != nil {
		return nil, fmt.Errorf("betfair stream dial: %w", err)
	}
	s := &Stream{conn: conn, reader: bufio.NewReader(conn)}

	if _, err := s.Read(); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.send(streamRequest{Op: "authentication", AppKey: c.appKey, Session: c.SessionToken()}); err != nil {
		s.Close()
		return nil, err
	}
	msg, err := s.Read()
	if err != nil {
		s.Close()
		return nil, err
	}
	if msg.Op != "status" || msg.StatusCode != "SUCCESS" {
		s.Close()
		return nil, fmt.Errorf("betfair stream auth: %s %s", msg.ErrorCode, msg.ErrorMessage)
	}
	return s, nil
}

// Subscribe replaces the stream's subscription with marketIDs, resuming from clk when given.
func (s *Stream) Subscribe(marketIDs []string, ladderLevels int, initialClk, clk string) error {
	return s.send(streamRequest{
		Op:           "marketSubscription",
		MarketFilter: &streamFilter{MarketIDs: marketIDs},
		MarketDataFilter: &marketDataFilter{
			Fields:       []string{"EX_BEST_OFFERS", "EX_LTP", "EX_MARKET_DEF"},
			LadderLevels: ladderLevels,
		},
		HeartbeatMs: heartbeatMs,
		InitialClk:  initialClk,
		Clk:         clk,
	})
}

// Read returns the next message, failing when nothing arrives within three heartbeats.
func (s *Stream) Read() (StreamMessage, error) {
	var msg StreamMessage
	if err := s.conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
		return msg, err
	}
	line, err := s.reader.ReadBytes('\n')
	if err != nil {
		return msg, fmt.Errorf("betfair stream read: %w", err)
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		return msg, fmt.Errorf("betfair stream decode: %w", err)
	}
	if msg.Op == "status" && msg.StatusCode == "FAILURE" {
		return msg, fmt.Errorf("betfair stream: %s %s", msg.ErrorCode, msg.ErrorMessage)
	}
	return msg, nil
}

func (s *Stream) Close() error { return s.conn.Close() }

func (s *Stream) send(req streamRequest) error {
	s.nextID++
	req.ID = s.nextID
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = s.conn.Write(append(b, '\r', '\n'))
	return err
}

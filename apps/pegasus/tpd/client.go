// packages/tpd/client.go

package tpd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"racing_wagering/apps/pegasus/core"

	logger "racing_wagering/logger"
)

const (
	readBufferSize = 64 * 1024
	raceImminent   = 5 * time.Minute

	quietCheckEvery = 5 * time.Minute

	// feed dark for a minute when race data expected
	quietFeedAfter = time.Minute
)

type Handler func(core.Update)

type Client struct {
	conn *net.UDPConn

	races   *GmaxClient
	tracker *Tracker
	handler Handler

	onFatal   func(error)
	fatalOnce sync.Once

	lastPacket atomic.Int64 // unix nanos
	dropped    atomic.Uint64
	received   atomic.Uint64
	firstSeen  sync.Once
}

func NewClient(ctx context.Context, port, licenceKey string, h Handler, onFatal func(error)) (*Client, error) {
	// pre-fetch races first
	// if no races loaded, stream listening is pointless
	races := NewGmaxClient(licenceKey)
	if err := races.run(ctx); err != nil {
		return nil, fmt.Errorf("tpd: race list: %w", err)
	}

	conn, err := listenUDP(port)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn:    conn,
		races:   races,
		tracker: NewTracker(races.Lookup),
		handler: h,
		onFatal: onFatal,
	}
	c.lastPacket.Store(time.Now().UnixNano())

	go c.read(ctx)
	go c.watchQuiet(ctx)
	go c.tracker.SweepExpired(ctx)

	logger.Info(logger.InfoLog{
		Message: fmt.Sprintf("tpd listening port=%s", port),
	})

	return c, nil
}

func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) read(ctx context.Context) {
	defer c.conn.Close()

	buf := make([]byte, readBufferSize)

	for {
		n, remote, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			// cancelled context closes the socket
			// error here is the expected way for loop to end
			select {
			case <-ctx.Done():
				return
			default:
			}

			logger.Error(logger.ErrorLog{
				Message: fmt.Sprintf("tpd udp read failed error=%v", err),
			})
			c.fatalOnce.Do(func() {
				if c.onFatal != nil {
					c.onFatal(err)
				}
			})
			return
		}

		c.lastPacket.Store(time.Now().UnixNano())
		c.received.Add(1)
		c.firstSeen.Do(func() {
			logger.Info(logger.InfoLog{
				Message: fmt.Sprintf("tpd first datagram received from=%v bytes=%v", remote, n),
			})
		})

		c.handleDatagram(buf[:n])
	}
}

func (c *Client) handleDatagram(datagram []byte) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(logger.ErrorLog{
				Message: fmt.Sprintf("tpd panic decoding a datagram; packet dropped panic=%v", r),
			})
		}
	}()

	// the tracker resolves identity; the packet rides along untouched
	bad := decodeDatagram(datagram, func(p Progress) {
		if ref, ok := c.tracker.Ref(p); ok {
			c.handler(core.Update{Ref: ref, Msg: p})
		}
	})

	// counted, not logged (2Hz)
	if bad > 0 {
		c.dropped.Add(uint64(bad))
	}
}

// silence only considered when a race is due to be running
func (c *Client) watchQuiet(ctx context.Context) {
	ticker := time.NewTicker(quietCheckEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			quiet := time.Since(time.Unix(0, c.lastPacket.Load()))
			if quiet < quietFeedAfter {
				continue
			}

			race := c.races.RaceDueWithin(raceImminent)
			if race == nil {
				continue
			}

			logger.Error(logger.ErrorLog{
				Message:     fmt.Sprintf("tpd feed silent with a race due quiet_for=%v race=%v scheduled_off=%v undecodable_packets=%v", quiet.Truncate(time.Second), race.Racecourse, race.PostTime.Format("2006-01-02 15:04 MST"), c.dropped.Load()),
				RaceDetails: &logger.RaceDetails{Venue: race.Racecourse, RaceNumber: race.RaceNumber},
			})
		}
	}
}

// decodeDatagram invokes fn for every progress packet in one datagram and
// returns how many payloads failed to decode. The feed sends either a JSON
// array or newline-delimited JSON; allocation is bounded by the datagram the
// socket already read.
func decodeDatagram(b []byte, fn func(Progress)) (bad int) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return 0
	}

	if b[0] == '[' {
		var batch []Progress
		if err := json.Unmarshal(b, &batch); err != nil {
			return 1
		}
		for _, p := range batch {
			if p.Kind == progressPacketKind {
				fn(p)
			}
		}
		return 0
	}

	for len(b) > 0 {
		line := b

		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			line, b = b[:i], b[i+1:]
		} else {
			b = nil
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var p Progress
		if err := json.Unmarshal(line, &p); err != nil {
			bad++
			continue
		}

		if p.Kind != progressPacketKind {
			continue
		}

		fn(p)
	}

	return bad
}

package exchange

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Bazcampbell/goreq"
)

// failingTransport answers every request with a 503 and counts them.
type failingTransport struct{ calls atomic.Int32 }

func (f *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls.Add(1)
	return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
}

func TestPlaceOrdersIsNeverRetried(t *testing.T) {
	transport := &failingTransport{}
	c := &Client{orderOptions: &goreq.Options{Client: &http.Client{Transport: transport}, Retries: 0}}

	if _, err := c.PlaceOrders(PlaceOrdersRequest{MarketID: "1.1"}); err == nil {
		t.Fatal("a 503 was reported as success")
	}
	if n := transport.calls.Load(); n != 1 {
		t.Fatalf("placeOrders sent %d requests, want 1", n)
	}
}

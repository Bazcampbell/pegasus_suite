package betmatic

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"pegasus_suite/betting"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	previous := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = previous })

	c := &Client{Email: "punter@example.com"}
	c.setToken("test-token")
	return c
}

func TestCreateNotificationRejectsNonSuccess(t *testing.T) {
	for _, status := range []int{400, 401, 402, 404, 500} {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"detail":"Subscription is not active"}`))
		})

		body, err := c.CreateNotification(NotificationRequest{Type: FIXED_PROFIT, Sports: "RACING"})
		if err == nil {
			t.Errorf("status %d was reported as a placed bet", status)
			continue
		}
		if !errors.Is(err, ErrNotificationRejected) {
			t.Errorf("status %d: got %v, want ErrNotificationRejected", status, err)
		}
		if body == "" {
			t.Errorf("status %d: response body was dropped", status)
		}
	}
}

func TestCreateNotificationAcceptsSuccess(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`OK`))
	})

	body, err := c.CreateNotification(NotificationRequest{Type: FIXED_PROFIT, Sports: "RACING"})
	if err != nil {
		t.Fatalf("a 200 was reported as a failure: %v", err)
	}
	if body != "OK" {
		t.Errorf("body = %q, want OK", body)
	}
}

func TestPlaceBetPropagatesRejection(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"detail":"Subscription is not active"}`))
	})

	var client betting.Client = c
	if err := client.PlaceBet(NotificationRequest{Type: FIXED_PROFIT, Sports: "RACING"}); err == nil {
		t.Error("PlaceBet reported success for a rejected notification")
	}
}

func TestCreateNotificationSendsAuthHeader(t *testing.T) {
	var got string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	if _, err := c.CreateNotification(NotificationRequest{Type: FIXED_PROFIT, Sports: "RACING"}); err != nil {
		t.Fatal(err)
	}
	if got != "Token test-token" {
		t.Errorf("Authorization = %q, want Token test-token", got)
	}
}

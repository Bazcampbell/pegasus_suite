// exchange/client.go

package exchange

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Bazcampbell/goreq"
)

const (
	authURL    = "https://identitysso-cert.betfair.com/api"
	bettingURL = "https://api.betfair.com/exchange/betting/rest/v1.0"

	httpTimeout = 3 * time.Second
	retries     = 2
	retryDelay  = 2 * time.Second

	sessionTokenLength = 44
)

type Client struct {
	token atomic.Pointer[string]

	Username string
	password string

	authOptions    *goreq.Options
	bettingOptions *goreq.Options
}

func New(username, password, appKey, cert string) (*Client, error) {
	c := &Client{Username: username, password: password}

	pem := []byte(cert)
	keyPair, err := tls.X509KeyPair(pem, pem)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{Certificates: []tls.Certificate{keyPair}},
		},
		Timeout: httpTimeout,
	}

	newOptions := func(contentType string) *goreq.Options {
		return &goreq.Options{
			Headers: map[string]string{
				"Accept":        "application/json",
				"X-Application": appKey,
				"Content-Type":  contentType,
			},
			Timeout:    httpTimeout,
			Retries:    retries,
			RetryDelay: retryDelay,
			Client:     httpClient,
		}
	}

	c.authOptions = newOptions("application/x-www-form-urlencoded")
	c.bettingOptions = newOptions("application/json")

	if err := c.Login(); err != nil {
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	return c, nil
}

func (c *Client) SessionToken() string {
	if t := c.token.Load(); t != nil {
		return *t
	}
	return ""
}

func (c *Client) setToken(t string) { c.token.Store(&t) }

func withToken(base *goreq.Options, token string) *goreq.Options {
	out := *base

	headers := make(map[string]string, len(base.Headers)+1)
	for k, v := range base.Headers {
		headers[k] = v
	}
	if token != "" {
		headers["X-Authentication"] = token
	}
	out.Headers = headers

	return &out
}

func (c *Client) Login() error {
	params := url.Values{}
	params.Add("username", c.Username)
	params.Add("password", c.password)

	resp, err := goreq.PostType[LoginResponse](
		authURL+"/certlogin",
		strings.NewReader(params.Encode()),
		withToken(c.authOptions, ""),
	)
	if err != nil {
		return err
	}

	if resp.Status != "SUCCESS" || len(resp.SessionToken) != sessionTokenLength {
		return fmt.Errorf("betfair login: %s", resp.Status)
	}

	c.setToken(resp.SessionToken)

	return nil
}

func (c *Client) KeepAlive() error {
	resp, err := goreq.PostType[KeepAliveResponse](authURL+"/keepAlive", nil, withToken(c.authOptions, c.SessionToken()))
	if err != nil {
		return err
	}

	if resp.Status != "SUCCESS" || len(resp.SessionToken) != sessionTokenLength {
		return fmt.Errorf("betfair keepAlive: %s %s", resp.Status, resp.Error)
	}

	c.setToken(resp.SessionToken)

	return nil
}

func (c *Client) Logout() error {
	resp, err := goreq.PostType[LogoutResponse](authURL+"/logout", nil, withToken(c.authOptions, c.SessionToken()))
	if err != nil {
		return err
	}

	if resp.Status != "SUCCESS" {
		return fmt.Errorf("betfair logout: %s %s", resp.Status, resp.Error)
	}

	c.setToken("")

	return nil
}

func post[T any](c *Client, endpoint string, body any) (T, error) {
	return goreq.PostType[T](bettingURL+"/"+endpoint+"/", body, withToken(c.bettingOptions, c.SessionToken()))
}

func (c *Client) ListEvents(filter MarketFilter) ([]ListEventsResponse, error) {
	return post[[]ListEventsResponse](c, "listEvents", ListRequest{Filter: filter})
}

func (c *Client) ListMarketCatalogue(req ListRequest) ([]MarketCatalogue, error) {
	return post[[]MarketCatalogue](c, "listMarketCatalogue", req)
}

func (c *Client) ListMarketBook(req ListMarketBookRequest) ([]MarketBook, error) {
	return post[[]MarketBook](c, "listMarketBook", req)
}

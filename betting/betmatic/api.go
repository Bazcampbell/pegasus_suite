// betmatic/api.go

package betmatic

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Bazcampbell/goreq"
	"github.com/google/go-querystring/query"
)

func (bc *Client) authHeaderOptions() *goreq.Options {
	return &goreq.Options{
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Token " + bc.getToken(),
		},
	}
}

func (bc *Client) authenticate() error {
	resp, err := goreq.PostType[AuthResponse](baseURL+"/account/login/", AuthRequest{
		Email:    bc.Email,
		Password: bc.password,
	}, nil)
	if err != nil {
		return err
	}

	bc.setToken(resp.Token)
	return nil
}

func (bc *Client) RefreshToken() error {
	resp, err := goreq.PostType[AuthResponse](baseURL+"/account/refresh_token/", AuthResponse{
		Token: bc.getToken(),
	}, bc.authHeaderOptions())
	if err != nil {
		return err
	}

	bc.setToken(resp.Token)
	return nil
}

var ErrNotificationRejected = errors.New("betmatic rejected the notification")

func (bc *Client) CreateNotification(req NotificationRequest) (string, error) {
	resp, err := goreq.Post(baseURL+"/notification/create/", req, bc.authHeaderOptions())
	if err != nil {
		if resp == nil {
			return "", fmt.Errorf("creating notification: %w", err)
		}
		return string(resp.Body), fmt.Errorf("creating notification: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return string(resp.Body), fmt.Errorf("creating notification: %w: status %d: %s", ErrNotificationRejected, resp.StatusCode, resp.Body)
	}

	return string(resp.Body), nil
}

func (bc *Client) GetNotifications(request GetNotificationsRequest) (GetNotificationResponse, error) {
	url := baseURL + "/notification/"

	queries, err := query.Values(request)
	if err != nil {
		return GetNotificationResponse{}, err
	}

	url = url + "?" + queries.Encode()

	return goreq.GetType[GetNotificationResponse](url, bc.authHeaderOptions())
}

func (bc *Client) GetNotificationBets(notificationID string) (NotificationBetsResponse, error) {
	url := baseURL + "/bet/notification/" + notificationID + "/"

	return goreq.GetType[NotificationBetsResponse](url, bc.authHeaderOptions())
}

// returns upcoming events based off a racing code and country code
func (bc *Client) GetUpcomingEvents(racingCode RacingCode, countryCode string) ([]Event, error) {
	resp, err := goreq.GetType[[]Event](baseURL+"/competition/namecodes/", bc.authHeaderOptions())
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(resp))
	for _, e := range resp {
		if e.Code == string(racingCode) && e.Country == countryCode {
			events = append(events, e)
		}
	}

	return events, nil
}

func (bc *Client) GetAllBookieAccounts() ([]BookieAccount, error) {
	return goreq.GetType[[]BookieAccount](baseURL+"/bookieaccount/", bc.authHeaderOptions())
}

// command is ON or OFF
func (bc *Client) controlBookieByID(id int, command BookieAccountCommand) error {
	_, err := goreq.PostJSON(baseURL+"/bookieaccount/control/"+strconv.Itoa(id)+"/", BookieAccountControlRequest{
		Command: string(command),
	}, bc.authHeaderOptions())
	return err
}

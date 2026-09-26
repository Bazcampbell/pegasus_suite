// betmatic/types.go

package betmatic

import (
	"strconv"
	"strings"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/platform/util"
)

type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

type Venue struct {
	Name    string
	IsMetro bool
}

type Bookmaker struct {
	ID                 int    `json:"id"`
	Title              string `json:"title"`
	MarketsUnavailable string `json:"markets_unavailable"`
	IconURI            string `json:"icon_uri"`
	Enabled            bool   `json:"enabled"`
	BonusEnabled       bool   `json:"bonus_enabled"`
	Note               string `json:"note"`
	Sports             bool   `json:"sports"`
	SportsBonusEnabled bool   `json:"sports_bonus_enabled"`
	Maintain           bool   `json:"maintain"`
	SupportManualLogin bool   `json:"support_manual_login"`
}

type Event struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	EventNumber int    `json:"event_number"`
	StartTime   string `json:"starttime"`
	Runners     string `json:"runners"`
	RunnerNames string `json:"runner_names"`
	Country     string `json:"country"`
	ScrapeDate  string `json:"scrape_date"`
}

type NotificationRequest struct {
	Type   BetType    `json:"type"`
	Market MarketType `json:"market,omitempty"`

	Sports          string     `json:"sports"`
	SportsMarket    string     `json:"sports_market,omitempty"`
	SportsSelection string     `json:"sports_selection,omitempty"`
	Competition     string     `json:"competition,omitempty"`
	Code            RacingCode `json:"code,omitempty"`
	EventNumber     int        `json:"event_number,omitempty"`
	EventID         string     `json:"event_id,omitempty"`
	Selection       int        `json:"selection,omitempty"`
	Handicap        float32    `json:"handicap,omitempty"`
	StartTime       string     `json:"start_time,omitempty"`
	ScheduledAt     string     `json:"scheduled_at,omitempty"`

	Stake        float64 `json:"stake,omitempty"`
	TargetProfit float64 `json:"target_profit,omitempty"`
	TotalWager   float64 `json:"total_wager,omitempty"`

	EnsureTotalWager bool `json:"ensure_total_wager,omitempty"`

	MinOdds      float32 `json:"odds,omitempty"`
	CheckOdds    bool    `json:"check_odds,omitempty"`
	MaxOdds      float32 `json:"max_odds,omitempty"`
	CheckMaxOdds bool    `json:"check_max_odds,omitempty"`

	TakeSP               bool `json:"take_sp,omitempty"`
	BoostIfAvailable     bool `json:"boost_if_available,omitempty"`
	SPGIfAvailable       bool `json:"spg_if_available,omitempty"`
	BonusBackIfAvailable bool `json:"bonus_back_if_available,omitempty"`
	UseBonus             bool `json:"use_bonus,omitempty"`

	AllowedDoubleBets bool `json:"allow_double_bets,omitempty"`

	ChooseBestOdds           bool `json:"choose_best_odds,omitempty"`
	ChooseRandomAccounts     bool `json:"choose_random_accounts,omitempty"`
	ChooseRandomBots         bool `json:"choose_random_bots,omitempty"`
	ChooseRandomBotDelayMode bool `json:"choose_random_bot_delay_mode,omitempty"`

	Bookies             string `json:"bookies,omitempty"`
	BookiesOverride     string `json:"bookies_override,omitempty"`
	TargetSession       string `json:"target_session,omitempty"`
	TargetBot           string `json:"target_bot,omitempty"`
	EnableGroupBot      bool   `json:"enable_group_bot,omitempty"`
	EnableGroupBotDelay bool   `json:"enable_group_bot_delay,omitempty"`

	OneAccountPerBookie bool `json:"one_account_per_bookie,omitempty"`

	Delay        int `json:"delay,omitempty"`
	SessionDelay int `json:"session_delay,omitempty"`
	BotDelay     int `json:"bot_delay,omitempty"`

	BotDelayMinTime int `json:"bot_delay_min_time,omitempty"`
	BotDelayMaxTime int `json:"bot_delay_max_time,omitempty"`

	DelayBetweenAttempts int `json:"delay_between_attempts,omitempty"`
	MaxAttempts          int `json:"max_attempts,omitempty"`

	IsScheduled  bool `json:"is_scheduled,omitempty"`
	IsMarketOpen bool `json:"is_market_open,omitempty"`
	AutoTrigger  bool `json:"auto_trigger,omitempty"`

	AcceptBetterLine bool `json:"accept_better_line,omitempty"`

	OnlyPlaceBetIfBetReturn float32 `json:"only_place_bet_if_bet_return,omitempty"`

	CustomStakeOption string `json:"custom_stake_option,omitempty"`
	SourceType        string `json:"source_type,omitempty"`
	TargetBetType     string `json:"target_bet_type,omitempty"`

	Label string `json:"label,omitempty"`
}

type NotificationResponse struct {
	Id      string `json:"id"`
	Message string `json:"message"`
}

type BookieAccountControlRequest struct {
	Command string `json:"command"`
}

type ControlBookieAccountRequest struct {
	Command string `json:"command"`
}

type GetNotificationResponse struct {
	Count      util.FlexInt `json:"count"`
	TotalPages util.FlexInt `json:"total_pages"`
	Next       string       `json:"next,omitempty"`
	Previous   string       `json:"previous,omitempty"`

	Results []Result `json:"results"`
}

type Result struct {
	ID  int64   `json:"id"`
	Tip Tip     `json:"tip"`
	Typ BetType `json:"type"`

	// What was asked for. Which one carries the request depends on the bet type:
	// Fixed Profit names a target and lets Betmatic size the stake.
	Stake        util.FlexFloat `json:"stake"`
	TargetProfit util.FlexFloat `json:"target_profit"`
	TotalWager   util.FlexFloat `json:"total_wager"`

	// What happened. TotalAccepted and Profit are dollars, and Profit is settled
	// P/L once the tip has a result: accepted × (average_odds − 1) on a winner.
	TotalAccepted util.FlexFloat `json:"total_accepted"`
	AverageOdds   util.FlexFloat `json:"average_odds"`
	Profit        util.FlexFloat `json:"profit"`

	IsCanceled  bool      `json:"is_canceled"`
	TargetBot   string    `json:"target_bot"`
	CreatedAt   time.Time `json:"created_at"`
	TriggeredAt time.Time `json:"triggered_at"`
	Label       string    `json:"label"`
}

func (r Result) Resulted() bool { return !r.Tip.ResultTime.IsZero() }

type Tip struct {
	Competition Competition `json:"competition"`
	Market      string      `json:"market"`
	Selection   string      `json:"selection"`

	MinOdds    util.FlexFloat `json:"odds"`
	Profit     util.FlexFloat `json:"profit"`
	ResultTime time.Time      `json:"result_time"`
}

type Competition struct {
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	EventNumber util.FlexFloat `json:"event_number"`

	Result      string `json:"result"`
	Runners     string `json:"runners"`
	RunnerNames string `json:"runner_names"`
}

func (c Competition) RunnerName(number int) string {
	numbers := strings.Split(c.Runners, ",")
	names := strings.Split(c.RunnerNames, "#&#")
	if len(numbers) != len(names) {
		return ""
	}
	for i, n := range numbers {
		if strings.TrimSpace(n) == strconv.Itoa(number) {
			return strings.TrimSpace(names[i])
		}
	}
	return ""
}

func (c Competition) Winner() int {
	first, _, _ := strings.Cut(c.Result, "/")
	n, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil {
		return 0
	}
	return n
}

type GetNotificationsRequest struct {
	Code     RacingCode `form:"code" url:"code,omitempty"`
	DateFrom string     `form:"date_from" url:"date_from,omitempty"`
	DateTo   string     `form:"date_to" url:"date_to,omitempty"`
	Label    string     `form:"label" url:"label,omitempty"`
	Market   MarketType `form:"market" url:"market,omitempty"`
	Sports   string     `form:"sports" url:"sports,omitempty"`
	Search   string     `form:"search" url:"search,omitempty"`

	MeetingDateFrom string `form:"meeting_date_from" url:"meeting_date_from,omitempty"`
	MeetingDateTo   string `form:"meeting_date_to" url:"meeting_date_to,omitempty"`

	Ordering string       `form:"ordering" url:"ordering,omitempty"`
	Page     util.FlexInt `form:"page" url:"page,omitempty"`
	PageSize util.FlexInt `form:"page_size" url:"page_size,omitempty"`
}

type NotificationBetsResponse struct {
	Bets []NotificationBet `json:"bets"`
}

type NotificationBet struct {
	ID           int64          `json:"id"`
	CreatedAt    string         `json:"created_at"`
	DisplayName  string         `json:"display_name"`
	Amount       util.FlexFloat `json:"amount"`
	CurrentOdds  util.FlexFloat `json:"current_odds"`
	SubmitResult string         `json:"submit_result"`
	SubmitError  string         `json:"submit_error"`
	Profit       util.FlexFloat `json:"profit"`
	Status       string         `json:"status"`
	Bookie       util.FlexInt   `json:"bookie"`
}

type BookieAccount struct {
	ID              int       `json:"id,omitempty"`
	Bookie          string    `json:"bookie,omitempty"`
	DisplayName     string    `json:"display_name,omitempty"`
	Balance         float64   `json:"balance,omitempty"`
	BonusBalance    float64   `json:"bonus_balance,omitempty"`
	UpdateTime      time.Time `json:"update_time,omitempty"`
	BotIP           string    `json:"bot_ip,omitempty"`
	BotID           string    `json:"bot_id,omitempty"`
	BotRunning      bool      `json:"bot_running,omitempty"`
	RunningSessions int       `json:"running_sessions,omitempty"`
}

func (r NotificationRequest) Provider() betting.Provider { return betting.ProviderBetmatic }

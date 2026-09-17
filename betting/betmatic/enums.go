// betmatic/enums.go

package betmatic

type RacingCode string
type MarketType string
type BetType string
type BookieAccountCommand string

const (
	FIXED_WAGER     BetType = "Fixed Wager"
	FIXED_PROFIT    BetType = "Fixed Profit"
	HIGH_ODDS_FIRST BetType = "High Odds First"
	SERIAL          BetType = "Serial"
)

const (
	FIXED_WIN       MarketType = "Fixed Win"
	FIXED_PLACE     MarketType = "Fixed Place"
	TOP_FLUC        MarketType = "Top Fluc"
	TOTE_WIN        MarketType = "Tote Win"
	TOTE_PLACE      MarketType = "Tote Place"
	BEST_TOTE_WIN   MarketType = "Best Tote Win"
	BEST_TOTE_PLACE MarketType = "Best Tote Place"
	MID_TOTE_WIN    MarketType = "Mid Tote Win"
	MID_TOTE_PLACE  MarketType = "Mid Tote Place"
	BEST_OF_BEST    MarketType = "Best of Best"
	SAME_RACE_MULTI MarketType = "Same Race Multi"
	BETFAIR_SP      MarketType = "Betfair SP"
	LAY_WIN         MarketType = "Lay Win"
	LAY_PLACE       MarketType = "Lay Place"
)

const (
	THOROUGHBRED RacingCode = "Galloping"
	HARNESS      RacingCode = "Harness"
	GREYHOUNDS   RacingCode = "Greyhounds"
)

const (
	ON  BookieAccountCommand = "ON"
	OFF BookieAccountCommand = "OFF"
)

// exchange/types.go

package exchange

import "time"

type MarketProjection string

const (
	MarketProjectionMarketStartTime   MarketProjection = "MARKET_START_TIME"
	MarketProjectionRunnerDescription MarketProjection = "RUNNER_DESCRIPTION"
	MarketProjectionRunnerMetadata    MarketProjection = "RUNNER_METADATA"
)

type MarketSort string

const MarketSortFirstToStart MarketSort = "FIRST_TO_START"

type PriceData string

const PriceDataExBestOffers PriceData = "EX_BEST_OFFERS"

type RollupModel string

const RollupModelStake RollupModel = "STAKE"

type MarketStatus string

const (
	MarketStatusInactive  MarketStatus = "INACTIVE"
	MarketStatusOpen      MarketStatus = "OPEN"
	MarketStatusSuspended MarketStatus = "SUSPENDED"
	MarketStatusClosed    MarketStatus = "CLOSED"
)

type Side string

const (
	SideBack Side = "BACK"
	SideLay  Side = "LAY"
)

type OrderType string

const (
	OrderTypeLimit         OrderType = "LIMIT"
	OrderTypeLimitOnClose  OrderType = "LIMIT_ON_CLOSE"
	OrderTypeMarketOnClose OrderType = "MARKET_ON_CLOSE"
)

type PersistenceType string

const (
	PersistenceTypeLapse         PersistenceType = "LAPSE"
	PersistenceTypePersist       PersistenceType = "PERSIST"
	PersistenceTypeMarketOnClose PersistenceType = "MARKET_ON_CLOSE"
)

type ExecutionReportStatus string

const (
	ExecutionReportStatusSuccess             ExecutionReportStatus = "SUCCESS"
	ExecutionReportStatusFailure             ExecutionReportStatus = "FAILURE"
	ExecutionReportStatusProcessedWithErrors ExecutionReportStatus = "PROCESSED_WITH_ERRORS"
	ExecutionReportStatusTimeout             ExecutionReportStatus = "TIMEOUT"
)

type InstructionReportStatus string

const (
	InstructionReportStatusSuccess InstructionReportStatus = "SUCCESS"
	InstructionReportStatusFailure InstructionReportStatus = "FAILURE"
	InstructionReportStatusTimeout InstructionReportStatus = "TIMEOUT"
)

type (
	ExecutionReportErrorCode   string
	InstructionReportErrorCode string
)

type LoginResponse struct {
	SessionToken string `json:"sessionToken"`
	Status       string `json:"loginStatus"`
}

type KeepAliveResponse struct {
	SessionToken string `json:"token"`
	Status       string `json:"status"`
	Error        string `json:"error"`
}

type LogoutResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type MarketFilter struct {
	EventTypeIds    []string   `json:"eventTypeIds,omitempty"`
	EventIds        []string   `json:"eventIds,omitempty"`
	MarketCountries []string   `json:"marketCountries,omitempty"`
	MarketTypeCodes []string   `json:"marketTypeCodes,omitempty"`
	MarketStartTime *TimeRange `json:"marketStartTime,omitempty"`
	BspOnly         *bool      `json:"bspOnly,omitempty"`
}

type ListRequest struct {
	MaxResults       int                `json:"maxResults,omitempty"`
	Filter           MarketFilter       `json:"filter,omitempty"`
	Sort             MarketSort         `json:"sort,omitempty"`
	MarketProjection []MarketProjection `json:"marketProjection,omitempty"`
}

type ListMarketBookRequest struct {
	MarketIds       []string        `json:"marketIds"`
	PriceProjection PriceProjection `json:"priceProjection,omitempty"`
}

type PriceProjection struct {
	PriceData  []PriceData           `json:"priceData,omitempty"`
	Overrides  ExBestOffersOverrides `json:"exBestOffersOverrides,omitempty"`
	Virtualise bool                  `json:"virtualise,omitempty"`
}

type ExBestOffersOverrides struct {
	BestPricesDepth int         `json:"bestPricesDepth,omitempty"`
	RollupModel     RollupModel `json:"rollupModel,omitempty"`
	RollupLimit     int         `json:"rollupLimit,omitempty"`
}

type Event struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CountryCode string    `json:"countryCode"`
	Timezone    string    `json:"timezone"`
	Venue       string    `json:"venue"`
	OpenDate    time.Time `json:"openDate"`
}

type ListEventsResponse struct {
	Event       Event `json:"event"`
	MarketCount int   `json:"marketCount"`
}

type MarketCatalogue struct {
	MarketID   string            `json:"marketId"`
	MarketName string            `json:"marketName"`
	Runners    []CatalogueRunner `json:"runners"`
}

type CatalogueRunner struct {
	SelectionID int64          `json:"selectionId"`
	RunnerName  string         `json:"runnerName"`
	Metadata    RunnerMetadata `json:"metadata"`
}

type RunnerMetadata struct {
	ClothNumber string `json:"CLOTH_NUMBER"`
}

type MarketBook struct {
	MarketID string       `json:"marketId"`
	Status   MarketStatus `json:"status"`
	Runners  []RunnerBook `json:"runners"`
}

type RunnerBook struct {
	SelectionID     int64          `json:"selectionId"`
	LastPriceTraded float64        `json:"lastPriceTraded"`
	EX              ExchangePrices `json:"ex"`
}

type ExchangePrices struct {
	AvailableToBack []PriceSize `json:"availableToBack"`
	AvailableToLay  []PriceSize `json:"availableToLay"`
}

type PriceSize struct {
	Price float64 `json:"price"`
	Size  float64 `json:"size"`
}

type PlaceOrdersRequest struct {
	MarketID            string             `json:"marketId"`
	Instructions        []PlaceInstruction `json:"instructions"`
	CustomerRef         string             `json:"customerRef,omitempty"`
	CustomerStrategyRef string             `json:"customerStrategyRef,omitempty"`
}

type PlaceInstruction struct {
	OrderType          OrderType           `json:"orderType"`
	SelectionID        int64               `json:"selectionId"`
	Side               Side                `json:"side"`
	Handicap           float64             `json:"handicap,omitempty"`
	LimitOrder         *LimitOrder         `json:"limitOrder,omitempty"`
	LimitOnCloseOrder  *LimitOnCloseOrder  `json:"limitOnCloseOrder,omitempty"`
	MarketOnCloseOrder *MarketOnCloseOrder `json:"marketOnCloseOrder,omitempty"`
	CustomerOrderRef   string              `json:"customerOrderRef,omitempty"`
}

type LimitOrder struct {
	Size            float64         `json:"size,omitempty"`
	Price           float64         `json:"price"`
	PersistenceType PersistenceType `json:"persistenceType,omitempty"`
}

type LimitOnCloseOrder struct {
	Liability float64 `json:"liability"`
	Price     float64 `json:"price"`
}

type MarketOnCloseOrder struct {
	Liability float64 `json:"liability"`
}

type PlaceExecutionReport struct {
	CustomerRef        string                   `json:"customerRef"`
	Status             ExecutionReportStatus    `json:"status"`
	ErrorCode          ExecutionReportErrorCode `json:"errorCode"`
	MarketID           string                   `json:"marketId"`
	InstructionReports []PlaceInstructionReport `json:"instructionReports"`
}

type PlaceInstructionReport struct {
	Status              InstructionReportStatus    `json:"status"`
	ErrorCode           InstructionReportErrorCode `json:"errorCode"`
	BetID               string                     `json:"betId"`
	PlacedDate          time.Time                  `json:"placedDate"`
	AveragePriceMatched float64                    `json:"averagePriceMatched"`
	SizeMatched         float64                    `json:"sizeMatched"`
}

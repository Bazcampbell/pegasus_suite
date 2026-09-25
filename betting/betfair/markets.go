// betfair/markets.go

package betfair

import (
	"fmt"
	"strconv"
	"time"

	"pegasus_suite/betting/betfair/internal/exchange"
	"pegasus_suite/logger"
)

func listEventsFilter(countryCodes []string) exchange.MarketFilter {
	now := time.Now()
	return exchange.MarketFilter{
		EventTypeIds:    []string{"7"},
		MarketCountries: countryCodes,
		MarketStartTime: &exchange.TimeRange{From: now.Add(streamFrom), To: now.Add(12 * time.Hour)},
	}
}

// listRaces returns the event's WIN markets keyed by race number.
func (bc *Client) listRaces(eventID string) (map[int]*Race, error) {
	markets, err := bc.api.ListMarketCatalogue(exchange.ListRequest{
		MaxResults: 100,
		Filter: exchange.MarketFilter{
			EventIds:        []string{eventID},
			MarketTypeCodes: []string{"WIN"},
		},
		Sort: exchange.MarketSortFirstToStart,
		MarketProjection: []exchange.MarketProjection{
			exchange.MarketProjectionMarketStartTime,
			exchange.MarketProjectionRunnerDescription,
			exchange.MarketProjectionRunnerMetadata,
		},
	})
	if err != nil {
		return nil, err
	}

	races := make(map[int]*Race, len(markets))
	for _, m := range markets {
		number, ok := parseRaceNumber(m.MarketName)
		if !ok {
			logger.Debug(logger.Log{Message: fmt.Sprintf("betfair market has no race number market_id=%v market_name=%v", m.MarketID, m.MarketName)})
			continue
		}
		races[number] = &Race{
			Number:    number,
			ID:        m.MarketID,
			Name:      m.MarketName,
			Distance:  parseDistance(m.MarketName),
			StartTime: m.MarketStartTime,
			Runners:   toRunners(m),
		}
	}
	return races, nil
}

// toRunners keys a market's runners by saddlecloth number, skipping any without one.
func toRunners(m exchange.MarketCatalogue) map[int]Runner {
	runners := make(map[int]Runner, len(m.Runners))
	for _, cr := range m.Runners {
		number, err := strconv.Atoi(cr.Metadata.ClothNumber)
		if err != nil || number == 0 {
			continue
		}
		runners[number] = Runner{Number: number, SelectionID: cr.SelectionID}
	}
	return runners
}

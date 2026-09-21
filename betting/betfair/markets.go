// betfair/markets.go

package betfair

import (
	"fmt"

	"strconv"
	"time"

	logger "pegasus_suite/logger"

	"pegasus_suite/betting/betfair/internal/exchange"
)

func listEventsFilter(countryCodes []string) exchange.MarketFilter {
	return exchange.MarketFilter{
		EventTypeIds:    []string{"7"},
		MarketCountries: countryCodes,
		MarketStartTime: &exchange.TimeRange{
			From: time.Now(),
			To:   time.Now().Add(12 * time.Hour),
		},
	}
}

// WIN markets for event, keyed by race number.
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

	logger.Debug(logger.Log{
		FormattedMessage: fmt.Sprintf("betfair markets returned event_id=%v market_count=%v", eventID, len(markets)),
	})

	races := make(map[int]*Race, len(markets))
	for _, m := range markets {
		number, ok := parseRaceNumber(m.MarketName)
		if !ok {
			logger.Debug(logger.Log{
				FormattedMessage: fmt.Sprintf("betfair market name has no race number, skipping event_id=%v market_id=%v market_name=%v", eventID, m.MarketID, m.MarketName),
			})
			continue
		}

		race := Race{
			Number:   number,
			ID:       m.MarketID,
			Name:     m.MarketName,
			Distance: parseDistance(m.MarketName),
			Runners:  toRunners(m),
		}
		races[number] = &race

		logger.Debug(logger.Log{
			FormattedMessage: fmt.Sprintf("betfair race parsed event_id=%v race_number=%v market_id=%v market_name=%v distance=%v runner_count=%v", eventID, number, race.ID, race.Name, race.Distance, len(race.Runners)),
		})
	}

	return races, nil
}

// runners keyed by cloth number, which is what selections are made on. the price
// feed is keyed by selection id instead, so both are kept on the runner.
func toRunners(m exchange.MarketCatalogue) map[int]*Runner {
	runners := make(map[int]*Runner, len(m.Runners))

	for _, cr := range m.Runners {
		number, err := strconv.Atoi(cr.Metadata.ClothNumber)
		if err != nil || number == 0 {
			logger.Debug(logger.Log{
				FormattedMessage: fmt.Sprintf("betfair runner has no cloth number, skipping market_id=%v selection_id=%v runner_name=%v", m.MarketID, cr.SelectionID, cr.RunnerName),
			})
			continue
		}

		runners[number] = &Runner{
			Number:      number,
			SelectionID: strconv.FormatInt(cr.SelectionID, 10),
			MarketID:    m.MarketID,
		}
	}

	return runners
}

func (bc *Client) listMarketBook(marketID string) ([]exchange.MarketBook, error) {
	return bc.api.ListMarketBook(exchange.ListMarketBookRequest{
		MarketIds: []string{marketID},
		PriceProjection: exchange.PriceProjection{
			PriceData: []exchange.PriceData{exchange.PriceDataExBestOffers},
			Overrides: exchange.ExBestOffersOverrides{
				BestPricesDepth: ORDER_BOOK_DEPTH,
				RollupModel:     exchange.RollupModelStake,
				RollupLimit:     0,
			},
		},
	})
}

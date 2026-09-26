// packages/core/mapping.go

package core

import (
	"pegasus_suite/betting"
	"pegasus_suite/betting/betfair"
	"pegasus_suite/betting/betmatic"
)

var TriplesRacingCodes = map[string]betmatic.RacingCode{
	"AUS/RVIC":  betmatic.THOROUGHBRED,
	"AUS/RSA":   betmatic.THOROUGHBRED,
	"AUS/RQLD":  betmatic.THOROUGHBRED,
	"AUS/HRNSW": betmatic.HARNESS,
	"AUS/HRVIC": betmatic.HARNESS,
	"AUS/HRQLD": betmatic.HARNESS,
}

func BetfairRacingCode(code betmatic.RacingCode) betfair.RacingCode {
	if code == betmatic.HARNESS {
		return betfair.TROT
	}
	return betfair.THOROUGHBRED
}

// lowercase and strip spaces key and value
var TriplesToBetfair = map[string]string{
	// THOROUGHS
	"aquisparkgoldcoast":         "goldcoast",
	"aquisparkgoldcoastpoly":     "goldcoast",
	"bendigo":                    "bendigo", // ALSO TROT KEY
	"balaklava":                  "balaklava",
	"caulfield":                  "caulfield",
	"caulfieldheath":             "caulfield",
	"dalby":                      "dalby",
	"doomben":                    "doomben",
	"eaglefarm":                  "eaglefarm",
	"flemington":                 "flemington",
	"gatton":                     "gatton",
	"ipswich":                    "ipswich",
	"kilcoy":                     "kilcoy",
	"ladbrokescannonpark":        "cairns",
	"ladbrokesgeelong":           "geelong",
	"mackay":                     "mackay",
	"mornington":                 "mornington",
	"morphettville":              "morphettville",
	"morphettvilleparks":         "morphettville",
	"picklebetparkwarwick":       "warwick",
	"rockhampton":                "rockhampton",
	"southsidecranbourne":        "cranbourne",
	"southsidepakenham":          "pakenham",
	"southsidepakenhamsynthetic": "pakenham",
	"sportsbetpakenhamsynthetic": "pakenham",
	"sportsbetgawler":            "gawler",
	"sportsbetsandownhillside":   "sandown",
	"sportsbetsandownlakeside":   "sandown",
	"sportsbetballarat":          "ballarat",
	"sportsbetballaratsynthetic": "ballarat",
	"sunshinecoast":              "sunshinecoast",
	"sunshinecoastpolytrack":     "sunshinecoast",
	"thevalley":                  "mooneevalley",
	"thomasfarmsrcmurraybridge":  "murraybridge",
	"toowoomba":                  "toowoomba",
	"townsville":                 "townsville",

	// TROTS
	"albionpark":          "albionpark",
	"albury":              "albury",
	"ballarat":            "ballarat",
	"bathurst":            "bathurst",
	"cranbourne":          "cranbourne",
	"geelong":             "geelong",
	"goulburn":            "goulburn",
	"kilmore":             "kilmore",
	"marburg":             "marburg",
	"melton":              "melton",
	"mildura":             "mildura",
	"newcastle":           "newcastle",
	"penrith":             "penrith",
	"redcliffe":           "redcliffe",
	"riverinapaceway":     "wagga", // TODO CHECK THIS
	"shepparton":          "shepparton",
	"tabcorpparkmenangle": "menangle",
	"tamworth":            "tamworth",
	"young":               "young",
}

var TriplesToBetmatic = map[string]BetmaticVenue{
	// THOROUGHBREDS
	"townsville": {
		Name:    "TOWNSVILLE",
		IsMetro: false,
	},
	"toowoomba": {
		Name:    "TOOWOOMBA",
		IsMetro: false,
	},
	"thevalley": {
		Name:    "MOONEE VALLEY",
		IsMetro: true,
	},
	"sunshinecoastpolytrack": {
		Name:    "SUNSHINE COAST",
		IsMetro: false,
	},
	"sunshinecoast": {
		Name:    "SUNSHINE COAST",
		IsMetro: false,
	},
	"sportsbetsandownlakeside": {
		Name:    "SANDOWN",
		IsMetro: true,
	},
	"sportsbetsandownhillside": {
		Name:    "SANDOWN",
		IsMetro: true,
	},
	"sportsbetpakenhamsynthetic": {
		Name:    "PAKENHAM",
		IsMetro: false,
	},
	"sportsbetballaratsynthetic": {
		Name:    "BALLARAT",
		IsMetro: false,
	},
	"sportsbetballarat": {
		Name:    "BALLARAT",
		IsMetro: false,
	},
	"sportsbetgawler": {
		Name:    "GAWLER",
		IsMetro: false,
	},
	"southsidepakenhamsynthetic": {
		Name:    "PAKENHAM",
		IsMetro: false,
	},
	"southsidepakenham": {
		Name:    "PAKENHAM",
		IsMetro: false,
	},
	"southsidecranbourne": {
		Name:    "CRANBOURNE",
		IsMetro: false,
	},
	"rockhampton": {
		Name:    "ROCKHAMPTON",
		IsMetro: false,
	},
	"picklebetparkwarwick": {
		Name:    "WARWICK",
		IsMetro: false,
	},
	"morphettvilleparks": {
		Name:    "MORPHETTVILLE",
		IsMetro: true,
	},
	"morphettville": {
		Name:    "MORPHETTVILLE",
		IsMetro: true,
	},
	"mornington": {
		Name:    "MORNINGTON",
		IsMetro: false,
	},
	"mackay": {
		Name:    "MACKAY",
		IsMetro: false,
	},
	"ladbrokesgeelong": {
		Name:    "GEELONG",
		IsMetro: false,
	},
	"ladbrokescannonpark": {
		Name:    "CAIRNS",
		IsMetro: false,
	},
	"kilcoy": {
		Name:    "KILCOY",
		IsMetro: false,
	},
	"ipswich": {
		Name:    "IPSWICH",
		IsMetro: false,
	},
	"gatton": {
		Name:    "GATTON",
		IsMetro: false,
	},
	"flemington": {
		Name:    "FLEMINGTON",
		IsMetro: true,
	},
	"eaglefarm": {
		Name:    "EAGLE FARM",
		IsMetro: true,
	},
	"doomben": {
		Name:    "DOOMBEN",
		IsMetro: true,
	},
	"dalby": {
		Name:    "DALBY",
		IsMetro: false,
	},
	"caulfieldheath": {
		Name:    "CAULFIELD",
		IsMetro: true,
	},
	"caulfield": {
		Name:    "CAULFIELD",
		IsMetro: true,
	},
	"balaklava": {
		Name:    "BALAKLAVA",
		IsMetro: false,
	},
	"thomasfarmsrcmurraybridge": {
		Name:    "MURRAY BRIDGE",
		IsMetro: false,
	},
	"bendigo": {
		Name:    "BENDIGO",
		IsMetro: false,
	},
	"aquisparkgoldcoastpoly": {
		Name:    "GOLD COAST",
		IsMetro: false,
	},
	"aquisparkgoldcoast": {
		Name:    "GOLD COAST",
		IsMetro: false,
	},

	// TROTS
	"albionpark": {
		Name:    "ALBION PARK",
		IsMetro: true,
	},
	"albury": {
		Name:    "ALBURY",
		IsMetro: false,
	},
	"ballarat": {
		Name:    "BALLARAT",
		IsMetro: false,
	},
	"bathurst": {
		Name:    "BATHURST",
		IsMetro: false,
	},
	//"bendigo": {
	//	Name:    "BENDIGO",
	//	IsMetro: false,
	//}, ** DUPLICATE, IRRELEVANT AS BETMATIC USES RACING CODE IN NOTIFICATION MESSAGES
	"cranbourne": {
		Name:    "CRANBOURNE",
		IsMetro: false,
	},
	"geelong": {
		Name:    "GEELONG",
		IsMetro: false,
	},
	"goulburn": {
		Name:    "GOULBURN",
		IsMetro: false,
	},
	"kilmore": {
		Name:    "KILMORE",
		IsMetro: false,
	},
	"marburg": {
		Name:    "MARBURG",
		IsMetro: false,
	},
	"melton": {
		Name:    "MELTON",
		IsMetro: true,
	},
	"mildura": {
		Name:    "MILDURA",
		IsMetro: false,
	},
	"newcastle": {
		Name:    "NEWCASTLE",
		IsMetro: false,
	},
	"penrith": {
		Name:    "PENRITH",
		IsMetro: false,
	},
	"redcliffe": {
		Name:    "REDCLIFFE",
		IsMetro: false,
	},
	"riverinapaceway": {
		Name:    "WAGGA",
		IsMetro: false,
	},
	"shepparton": {
		Name:    "SHEPPARTON",
		IsMetro: false,
	},
	"tabcorpparkmenangle": {
		Name:    "MENANGLE",
		IsMetro: true,
	},
	"tamworth": {
		Name:    "TAMWORTH",
		IsMetro: false,
	},
	"young": {
		Name:    "YOUNG",
		IsMetro: false,
	},
}

// BetfairTrackFor returns the Betfair track name for a Triple-S venue.
func BetfairTrackFor(venue string) (string, bool) {
	track, ok := TriplesToBetfair[betting.NormaliseTrackKey(venue)]
	return track, ok
}

// BetmaticVenueFor returns the Betmatic venue for a Triple-S venue.
func BetmaticVenueFor(venue string) (BetmaticVenue, bool) {
	v, ok := TriplesToBetmatic[betting.NormaliseTrackKey(venue)]
	return v, ok
}

// CanonicalVenue returns the Betmatic name for a Triple-S venue, or feedName when it has none.
func CanonicalVenue(venue, feedName string) string {
	if v, ok := BetmaticVenueFor(venue); ok {
		return v.Name
	}
	return feedName
}

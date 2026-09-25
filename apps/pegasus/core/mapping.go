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

var TPDToBetfair = map[string]string{
	// US
	"98": "belmontpark",           // Belmont Park
	"82": "canterburypark",        // Canterbury Park
	"97": "churchilldowns",        // Churchill Downs
	"78": "colonialdowns",         // Colonial Downs
	"79": "delmar",                // Del Mar
	"EP": "ellispark",             // Ellis Park
	"FG": "fairgrounds",           // Fair Grounds
	"86": "gulfstreampark",        // Gulfstream Park
	"89": "horseshoeindianapolis", // Horseshoe Indianapolis
	"75": "keeneland",             // Keeneland
	"73": "laurelpark",            // Laurel Park
	"70": "louisianadowns",        // Louisiana Downs
	"84": "monmouthpark",          // Monmouth Park
	"81": "oaklawnpark",           // Oaklawn Park
	"74": "pimlico",               // Pimlico
	"80": "samhouston",            // Sam Houston
	"94": "santaanita",            // Santa Anita
	"ST": "saratoga",              // Saratoga
	"85": "tampabaydowns",         // Tampa Bay Downs
	"87": "meadowlands",           // Meadowlands
	"88": "hoosierpark",           // Hooiser Park
	"95": "turfwaypark",           // Turfway Park

	// CAN
	"90": "woodbine", // Woodbine
	"91": "woodbine", // Woodbine Mohawk Park

	// UK
	"01": "ascot",         // Ascot (flat), Ascot (jump)
	"03": "bangor",        // Bangor-On-Dee
	"04": "bath",          // Bath
	"06": "brighton",      // Brighton
	"11": "chepstow",      // Chepstow
	"12": "chester",       // Chester
	"14": "doncaster",     // Doncaster
	"17": "fakenham",      // Fakenham
	"64": "ffoslas",       // Ffos Las
	"19": "fontwell",      // Fontwell Park
	"23": "hereford",      // Hereford
	"24": "hexham",        // Hexham
	"30": "lingfield",     // Lingfield Park
	"34": "newbury",       // Newbury
	"35": "newcastle",     // Newcastle
	"37": "newtonabbot",   // Newton Abbot
	"40": "plumpton",      // Plumpton
	"43": "ripon",         // Ripon
	"46": "sedgefield",    // Sedgefield
	"47": "southwell",     // Southwell
	"53": "uttoxeter",     // Uttoxeter
	"57": "windsor",       // Windsor
	"58": "wolverhampton", // Wolverhampton
	"59": "worcester",     // Worcester
	"61": "yarmouth",      // Yarmouth
	"31": "aintree",       // Aintree
	"02": "ayr",           // Ayr
	"05": "beverley",      // Beverley
	"07": "carlisle",      // Carlisle
	"08": "cartmel",       // Cartmel
	"09": "catterick",     // Catterick Bridge
	"65": "greatleighs",   // Chelmsford
	"10": "cheltenham",    // Cheltenham
	"16": "epsom",         // Epsom Downs
	"13": "exeter",        // Exeter
	"20": "goodwood",      // Goodwood
	"21": "hamilton",      // Hamilton Park
	"22": "haydock",       // Haydock Park
	"25": "huntingdon",    // Huntingdon
	"26": "kelso",         // Kelso
	"27": "kempton",       // Kempton Park
	"29": "leicester",     // Leicester
	"32": "ludlow",        // Ludlow
	"33": "marketrasen",   // Market Rasen
	"15": "musselburgh",   // Musselburgh
	"36": "newmarket",     // Newmarket
	"38": "nottingham",    // Nottingham
	"39": "perth",         // Perth
	"41": "pontefract",    // Pontefract
	"42": "redcar",        // Redcar
	"44": "salisbury",     // Salisbury
	"45": "sandown",       // Sandown Park
	"48": "stratford",     // Stratford-On-Avon
	"49": "taunton",       // Taunton
	"51": "thirsk",        // Thirsk
	"54": "warwick",       // Warwick
	"55": "wetherby",      // Wetherby
	"56": "wincanton",     // Wincanton
	"62": "york",          // York
}

var TPDToBetmatic = map[string]BetmaticVenue{
	// THOROUGHBREDS

	// --- US ---

	// Belmont Park
	"98": {
		Name:    "BELMONT PARK USA",
		IsMetro: false,
	},
	// Canterbury Park
	"82": {
		Name:    "CANTERBURY PARK USA",
		IsMetro: false,
	},
	// Churchill Downs
	"97": {
		Name:    "CHURCHILL DOWNS",
		IsMetro: false,
	},
	// Colonial Downs
	"78": {
		Name:    "COLONIAL DOWNS",
		IsMetro: false,
	},
	// Del Mar
	"79": {
		Name:    "DEL MAR",
		IsMetro: false,
	},
	// Ellis Park
	"EP": {
		Name:    "ELLIS PARK",
		IsMetro: false,
	},
	// Fair Grounds
	"FG": {
		Name:    "FAIR GROUNDS",
		IsMetro: false,
	},
	// Gulfstream Park
	"86": {
		Name:    "GULFSTREAM PARK",
		IsMetro: false,
	},
	// Horseshoe Indianapolis
	"89": {
		Name:    "HORSESHOE INDIANAPOLIS",
		IsMetro: false,
	},
	// Keeneland
	"75": {
		Name:    "KEENELAND",
		IsMetro: false,
	},
	// Laurel Park
	"73": {
		Name:    "LAUREL PARK",
		IsMetro: false,
	},
	// Louisiana Downs
	"70": {
		Name:    "LOUISIANA DOWNS",
		IsMetro: false,
	},
	// Monmouth Park
	"84": {
		Name:    "MONMOUTH PARK",
		IsMetro: false,
	},
	// Oaklawn Park
	"81": {
		Name:    "OAKLAWN",
		IsMetro: false,
	},
	// Pimlico
	"74": {
		Name:    "PIMLICO",
		IsMetro: false,
	},
	// Sam Houston
	"80": {
		Name:    "SAM HOUSTON",
		IsMetro: false,
	},
	// Santa Anita
	"94": {
		Name:    "SANTA ANITA PARK",
		IsMetro: false,
	},
	// Saratoga
	"ST": {
		Name:    "SARATOGA",
		IsMetro: false,
	},
	// Tampa Bay Downs
	"85": {
		Name:    "TAMPA BAY DOWNS",
		IsMetro: false,
	},
	// Meadowlands
	"87": {
		Name:    "MEADOWLANDS",
		IsMetro: false,
	},
	// Hooiser Park (sic - Hoosier Park)
	"88": {
		Name:    "HOOSIER PARK",
		IsMetro: false,
	},
	// Turfway Park
	"95": {
		Name:    "TURFWAY PARK",
		IsMetro: false,
	},

	// --- CANADA ---

	// Woodbine
	"90": {
		Name:    "WOODBINE",
		IsMetro: false,
	},
	// Woodbine Mohawk Park
	"91": {
		Name:    "MOHAWK",
		IsMetro: false,
	},

	// --- UK ---

	// Ascot (flat) / Ascot (jump) - both share ID 1
	"1": {
		Name:    "ASCOT UK",
		IsMetro: false,
	},
	// Ayr
	"2": {
		Name:    "AYR",
		IsMetro: false,
	},
	// Bangor-On-Dee
	"3": {
		Name:    "BANGOR ON DEE",
		IsMetro: false,
	},
	// Bath
	"4": {
		Name:    "BATH",
		IsMetro: false,
	},
	// Beverley
	"5": {
		Name:    "BEVERLEY",
		IsMetro: false,
	},
	// Brighton
	"6": {
		Name:    "BRIGHTON",
		IsMetro: false,
	},
	// Carlisle
	"7": {
		Name:    "CARLISLE",
		IsMetro: false,
	},
	// Cartmel
	"8": {
		Name:    "CARTMEL",
		IsMetro: false,
	},
	// Catterick Bridge
	"9": {
		Name:    "CATTERICK BRIDGE",
		IsMetro: false,
	},
	// Cheltenham
	"10": {
		Name:    "CHELTENHAM UK",
		IsMetro: false,
	},
	// Chepstow
	"11": {
		Name:    "CHEPSTOW",
		IsMetro: false,
	},
	// Chester
	"12": {
		Name:    "CHESTER",
		IsMetro: false,
	},
	// Exeter
	"13": {
		Name:    "EXETER",
		IsMetro: false,
	},
	// Doncaster
	"14": {
		Name:    "DONCASTER",
		IsMetro: false,
	},
	// Musselburgh
	"15": {
		Name:    "MUSSELBURGH",
		IsMetro: false,
	},
	// Epsom Downs
	"16": {
		Name:    "EPSOM DOWNS",
		IsMetro: false,
	},
	// Fakenham
	"17": {
		Name:    "FAKENHAM",
		IsMetro: false,
	},
	// Fontwell Park
	"19": {
		Name:    "FONTWELL PARK",
		IsMetro: false,
	},
	// Goodwood
	"20": {
		Name:    "GOODWOOD",
		IsMetro: false,
	},
	// Hamilton Park
	"21": {
		Name:    "HAMILTON PARK",
		IsMetro: false,
	},
	// Haydock Park
	"22": {
		Name:    "HAYDOCK PARK",
		IsMetro: false,
	},
	// Hereford
	"23": {
		Name:    "HEREFORD",
		IsMetro: false,
	},
	// Hexham
	"24": {
		Name:    "HEXHAM",
		IsMetro: false,
	},
	// Huntingdon
	"25": {
		Name:    "HUNTINGDON",
		IsMetro: false,
	},
	// Kelso
	"26": {
		Name:    "KELSO",
		IsMetro: false,
	},
	// Kempton Park
	"27": {
		Name:    "KEMPTON PARK",
		IsMetro: false,
	},
	// Leicester
	"29": {
		Name:    "LEICESTER",
		IsMetro: false,
	},
	// Lingfield Park
	"30": {
		Name:    "LINGFIELD",
		IsMetro: false,
	},
	// Aintree
	"31": {
		Name:    "AINTREE",
		IsMetro: false,
	},
	// Ludlow
	"32": {
		Name:    "LUDLOW",
		IsMetro: false,
	},
	// Market Rasen
	"33": {
		Name:    "MARKET RASEN",
		IsMetro: false,
	},
	// Newbury
	"34": {
		Name:    "NEWBURY",
		IsMetro: false,
	},
	// Newcastle
	"35": {
		Name:    "NEWCASTLE UK",
		IsMetro: false,
	},
	// Newmarket
	"36": {
		Name:    "NEWMARKET UK",
		IsMetro: false,
	},
	// Newton Abbot
	"37": {
		Name:    "NEWTON ABBOT",
		IsMetro: false,
	},
	// Nottingham
	"38": {
		Name:    "NOTTINGHAM",
		IsMetro: false,
	},
	// Perth
	"39": {
		Name:    "PERTH UK",
		IsMetro: false,
	},
	// Plumpton
	"40": {
		Name:    "PLUMPTON",
		IsMetro: false,
	},
	// Pontefract
	"41": {
		Name:    "PONTEFRACT",
		IsMetro: false,
	},
	// Redcar
	"42": {
		Name:    "REDCAR",
		IsMetro: false,
	},
	// Ripon
	"43": {
		Name:    "RIPON",
		IsMetro: false,
	},
	// Salisbury
	"44": {
		Name:    "SALISBURY",
		IsMetro: false,
	},
	// Sandown Park
	"45": {
		Name:    "SANDOWN PARK UK",
		IsMetro: false,
	},
	// Sedgefield
	"46": {
		Name:    "SEDGEFIELD",
		IsMetro: false,
	},
	// Southwell
	"47": {
		Name:    "SOUTHWELL",
		IsMetro: false,
	},
	// Stratford-On-Avon
	"48": {
		Name:    "STRATFORD AVON",
		IsMetro: false,
	},
	// Taunton
	"49": {
		Name:    "TAUNTON",
		IsMetro: false,
	},
	// Thirsk
	"51": {
		Name:    "THIRSK",
		IsMetro: false,
	},
	// Uttoxeter
	"53": {
		Name:    "UTTOXETER",
		IsMetro: false,
	},
	// Warwick
	"54": {
		Name:    "WARWICK UK",
		IsMetro: false,
	},
	// Wetherby
	"55": {
		Name:    "WETHERBY",
		IsMetro: false,
	},
	// Wincanton
	"56": {
		Name:    "WINCANTON",
		IsMetro: false,
	},
	// Windsor
	"57": {
		Name:    "WINDSOR",
		IsMetro: false,
	},
	// Wolverhampton
	"58": {
		Name:    "WOLVERHAMPTON",
		IsMetro: false,
	},
	// Worcester
	"59": {
		Name:    "WORCESTER",
		IsMetro: false,
	},
	// Yarmouth
	"61": {
		Name:    "YARMOUTH",
		IsMetro: false,
	},
	// York
	"62": {
		Name:    "YORK",
		IsMetro: false,
	},
	// Ffos Las
	"64": {
		Name:    "FFOS LAS",
		IsMetro: false,
	},
	// Chelmsford
	"65": {
		Name:    "CHELMSFORD CITY",
		IsMetro: false,
	},
}

// live feed for country
func ProviderFor(country string) (Provider, bool) {
	switch country {
	case "AU":
		return ProviderTripleS, true
	case "GB", "US", "CA", "SA", "CL", "IN":
		return ProviderTPD, true
	}
	return "", false
}

// agnostic mapping
func BetfairTrackFor(provider Provider, venue string) (string, bool) {
	switch provider {
	case ProviderTripleS:
		track, ok := TriplesToBetfair[betting.NormaliseTrackKey(venue)]
		return track, ok
	case ProviderTPD:
		track, ok := TPDToBetfair[venue]
		return track, ok
	}
	return "", false
}

func BetmaticVenueFor(provider Provider, venue string) (BetmaticVenue, bool) {
	switch provider {
	case ProviderTripleS:
		v, ok := TriplesToBetmatic[betting.NormaliseTrackKey(venue)]
		return v, ok
	case ProviderTPD:
		// Keyed by the TPD course code, the same as TPDToBetfair — not by the
		// betfair track name, which would miss every row.
		v, ok := TPDToBetmatic[venue]
		return v, ok
	}
	return BetmaticVenue{}, false
}

// CanonicalVenue is the one name a race is logged under: the Betmatic name
// when the venue maps, else what the feed called it. Resolved once per packet
// at fan-out so nothing downstream has to.
func CanonicalVenue(provider Provider, venue, feedName string) string {
	if v, ok := BetmaticVenueFor(provider, venue); ok {
		return v.Name
	}
	return feedName
}

// betfairToBetmatic is the two feed tables joined on their shared key, so a
// betfair track name can be logged under the canonical (Betmatic) name.
var betfairToBetmatic = func() map[string]string {
	m := make(map[string]string, len(TriplesToBetfair)+len(TPDToBetfair))
	for key, track := range TriplesToBetfair {
		if v, ok := TriplesToBetmatic[key]; ok {
			m[track] = v.Name
		}
	}
	for code, track := range TPDToBetfair {
		if v, ok := TPDToBetmatic[code]; ok {
			m[track] = v.Name
		}
	}
	return m
}()

// BetmaticNameForBetfair returns the canonical name for a betfair track.
func BetmaticNameForBetfair(track string) (string, bool) {
	name, ok := betfairToBetmatic[betting.NormaliseTrackKey(track)]
	return name, ok
}

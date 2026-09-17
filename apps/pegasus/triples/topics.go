// triple-s/topics.go

package triples

// TopicCode returns the COUNTRY/CODE pair from a triple-s topic —
// "PROD/PRDF/AUS/RVIC/FLEMINGTON/3" → "AUS/RVIC" — or "" if the topic is
// shorter than that. Levels 0-1 are the environment prefix.
//
// This runs on every inbound message, so it scans once and slices rather than
// splitting: no allocation, no wildcard matching. Process routing compares the
// result by equality.
func TopicCode(topic string) string {
	start, end, slashes := -1, -1, 0

	for i := 0; i < len(topic); i++ {
		if topic[i] != '/' {
			continue
		}

		slashes++
		if slashes == 2 {
			start = i + 1
		} else if slashes == 4 {
			end = i
			break
		}
	}

	if slashes < 3 {
		return ""
	}

	if end < 0 {
		end = len(topic)
	}

	return topic[start:end]
}

// Topics is every feed Triple-S publishes for Australia. It is a constant
// rather than a setting: the connection is shared by every process, so
// subscribing to less than everything only ever means a scope silently
// receives nothing. What actually gets bet is decided by the scope stakes.
var Topics = []string{
	"PROD/PRDF/AUS/RVIC/#",
	"PROD/PRDF/AUS/RSA/#",
	"PROD/PRDF/AUS/RQLD/#",
	"PROD/PRDF/AUS/HRNSW/#",
	"PROD/PRDF/AUS/HRVIC/#",
	"PROD/PRDF/AUS/HRQLD/#",
}

// core/genericTypes.go

package core

import "racing_wagering/platform/util"

// triple-s sends "NaN" for a pre-race distance and quotes some numbers as
// strings; these tolerate both. Aliases rather than copies so the betmatic and
// betfair types in the SDK decode identically.
type (
	FlexFloat = util.FlexFloat
	FlexInt   = util.FlexInt
)

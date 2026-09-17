// util/flex.go

package util

import (
	"math"
	"strconv"
	"strings"
)

type FlexFloat float64

func (f *FlexFloat) UnmarshalJSON(b []byte) error {
	v, err := parseNumber(b)
	if err != nil {
		return err
	}
	*f = FlexFloat(v)
	return nil
}

type FlexInt int

func (i *FlexInt) UnmarshalJSON(b []byte) error {
	v, err := parseNumber(b)
	if err != nil {
		return err
	}
	*i = FlexInt(int64(v))
	return nil
}

func parseNumber(b []byte) (float64, error) {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return 0, nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = strings.TrimSpace(s[1 : len(s)-1])
		if s == "" {
			return 0, nil
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, nil
	}
	return v, nil
}

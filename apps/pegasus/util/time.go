// packages/util/time.go

package util

import "time"

const ISO_8601_FORMAT = "2006-01-02T15:04:05Z"

func ISO8601ToTime(timeStr string) (time.Time, error) {
	t, err := time.Parse(ISO_8601_FORMAT, timeStr)
	if err != nil {
		return time.Time{}, err
	}

	return t, nil
}

package timecode

import "time"

func unixNanoUTC(value int64) time.Time {
	return time.Unix(0, value).UTC()
}

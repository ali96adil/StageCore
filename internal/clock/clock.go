package clock

import "time"

type Clock interface {
	Now() time.Time
}

type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

type Fixed struct{ Time time.Time }

func (f Fixed) Now() time.Time { return f.Time.UTC() }

func UnixMicros(t time.Time) int64     { return t.UTC().UnixMicro() }
func FromUnixMicros(v int64) time.Time { return time.UnixMicro(v).UTC() }

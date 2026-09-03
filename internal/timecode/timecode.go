package timecode

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type SourceKind string

const (
	SourceInternal SourceKind = "INTERNAL"
	SourceMTC      SourceKind = "MTC"
	SourceLTC      SourceKind = "LTC"
)

type TransportState string

const (
	TransportStopped TransportState = "STOPPED"
	TransportRunning TransportState = "RUNNING"
	TransportPaused  TransportState = "PAUSED"
)

type Rate struct {
	Name        string `json:"name"`
	Numerator   int64  `json:"numerator"`
	Denominator int64  `json:"denominator"`
	NominalFPS  int    `json:"nominal_fps"`
	DropFrame   bool   `json:"drop_frame"`
}

var (
	Rate23976    = Rate{Name: "23.976", Numerator: 24000, Denominator: 1001, NominalFPS: 24}
	Rate24       = Rate{Name: "24", Numerator: 24, Denominator: 1, NominalFPS: 24}
	Rate25       = Rate{Name: "25", Numerator: 25, Denominator: 1, NominalFPS: 25}
	Rate2997     = Rate{Name: "29.97", Numerator: 30000, Denominator: 1001, NominalFPS: 30}
	Rate2997Drop = Rate{Name: "29.97 DF", Numerator: 30000, Denominator: 1001, NominalFPS: 30, DropFrame: true}
	Rate30       = Rate{Name: "30", Numerator: 30, Denominator: 1, NominalFPS: 30}
	Rate5994     = Rate{Name: "59.94", Numerator: 60000, Denominator: 1001, NominalFPS: 60}
	Rate5994Drop = Rate{Name: "59.94 DF", Numerator: 60000, Denominator: 1001, NominalFPS: 60, DropFrame: true}
	Rate60       = Rate{Name: "60", Numerator: 60, Denominator: 1, NominalFPS: 60}
)

func SupportedRates() []Rate {
	return []Rate{Rate23976, Rate24, Rate25, Rate2997, Rate2997Drop, Rate30, Rate5994, Rate5994Drop, Rate60}
}

func ParseRate(name string) (Rate, error) {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "DROP-FRAME", "DF")
	normalized = strings.ReplaceAll(normalized, "DROP FRAME", "DF")
	normalized = strings.Join(strings.Fields(normalized), " ")
	aliases := map[string]Rate{
		"23.976": Rate23976, "24000/1001": Rate23976,
		"24": Rate24, "24 FPS": Rate24,
		"25": Rate25, "25 FPS": Rate25,
		"29.97": Rate2997, "30000/1001": Rate2997,
		"29.97 DF": Rate2997Drop, "30000/1001 DF": Rate2997Drop,
		"30": Rate30, "30 FPS": Rate30,
		"59.94": Rate5994, "60000/1001": Rate5994,
		"59.94 DF": Rate5994Drop, "60000/1001 DF": Rate5994Drop,
		"60": Rate60, "60 FPS": Rate60,
	}
	if rate, ok := aliases[normalized]; ok {
		return rate, nil
	}
	return Rate{}, fmt.Errorf("unsupported timecode rate %q", name)
}

func (r Rate) Validate() error {
	if r.Numerator <= 0 || r.Denominator <= 0 || r.NominalFPS <= 0 {
		return errors.New("invalid frame rate")
	}
	if !r.DropFrame {
		return nil
	}
	if r.Denominator != 1001 || (r.Numerator != 30000 && r.Numerator != 60000) {
		return errors.New("drop-frame is supported only for 29.97 and 59.94 fps")
	}
	return nil
}

func (r Rate) DropFramesPerMinute() int64 {
	if !r.DropFrame {
		return 0
	}
	return int64(r.NominalFPS / 15)
}

func (r Rate) MTCCode() (byte, bool) {
	switch {
	case r == Rate24:
		return 0, true
	case r == Rate25:
		return 1, true
	case r == Rate2997Drop:
		return 2, true
	case r == Rate30:
		return 3, true
	default:
		return 0, false
	}
}

func RateFromMTCCode(code byte) (Rate, error) {
	switch code & 0x03 {
	case 0:
		return Rate24, nil
	case 1:
		return Rate25, nil
	case 2:
		return Rate2997Drop, nil
	case 3:
		return Rate30, nil
	default:
		panic("unreachable")
	}
}

type Timecode struct {
	Hours   int  `json:"hours"`
	Minutes int  `json:"minutes"`
	Seconds int  `json:"seconds"`
	Frames  int  `json:"frames"`
	Rate    Rate `json:"rate"`
}

func (tc Timecode) Validate() error {
	if err := tc.Rate.Validate(); err != nil {
		return err
	}
	if tc.Hours < 0 || tc.Hours > 23 || tc.Minutes < 0 || tc.Minutes > 59 || tc.Seconds < 0 || tc.Seconds > 59 {
		return errors.New("timecode clock component out of range")
	}
	if tc.Frames < 0 || tc.Frames >= tc.Rate.NominalFPS {
		return errors.New("timecode frame component out of range")
	}
	if tc.Rate.DropFrame && tc.Seconds == 0 && tc.Minutes%10 != 0 && int64(tc.Frames) < tc.Rate.DropFramesPerMinute() {
		return errors.New("timecode uses a dropped frame number")
	}
	return nil
}

func (tc Timecode) String() string {
	sep := ":"
	if tc.Rate.DropFrame {
		sep = ";"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%02d", tc.Hours, tc.Minutes, tc.Seconds, sep, tc.Frames)
}

func Parse(value string, rate Rate) (Timecode, error) {
	normalized := strings.TrimSpace(value)
	if strings.Contains(normalized, ";") != rate.DropFrame {
		return Timecode{}, errors.New("timecode delimiter does not match drop-frame mode")
	}
	replacer := strings.NewReplacer(";", ":")
	parts := strings.Split(replacer.Replace(normalized), ":")
	if len(parts) != 4 {
		return Timecode{}, errors.New("timecode must use HH:MM:SS:FF or HH:MM:SS;FF")
	}
	values := make([]int, 4)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return Timecode{}, errors.New("timecode contains a non-numeric component")
		}
		values[i] = n
	}
	tc := Timecode{Hours: values[0], Minutes: values[1], Seconds: values[2], Frames: values[3], Rate: rate}
	if err := tc.Validate(); err != nil {
		return Timecode{}, err
	}
	return tc, nil
}

func (tc Timecode) FrameNumber() (int64, error) {
	if err := tc.Validate(); err != nil {
		return 0, err
	}
	nominal := int64(tc.Rate.NominalFPS)
	base := (int64(tc.Hours)*3600+int64(tc.Minutes)*60+int64(tc.Seconds))*nominal + int64(tc.Frames)
	if !tc.Rate.DropFrame {
		return base, nil
	}
	totalMinutes := int64(tc.Hours*60 + tc.Minutes)
	dropped := tc.Rate.DropFramesPerMinute() * (totalMinutes - totalMinutes/10)
	return base - dropped, nil
}

func FromFrameNumber(frame int64, rate Rate) (Timecode, error) {
	if err := rate.Validate(); err != nil {
		return Timecode{}, err
	}
	if frame < 0 {
		return Timecode{}, errors.New("negative frame number")
	}
	nominal := int64(rate.NominalFPS)
	framesPer24Hours := nominal * 60 * 60 * 24
	displayFrame := frame
	if rate.DropFrame {
		drop := rate.DropFramesPerMinute()
		framesPer10Minutes := nominal*60*10 - drop*9
		framesPer24Hours = framesPer10Minutes * 6 * 24
		frame %= framesPer24Hours
		tenMinuteBlocks := frame / framesPer10Minutes
		remainder := frame % framesPer10Minutes
		displayFrame = frame + drop*9*tenMinuteBlocks
		framesPerMinute := nominal*60 - drop
		if remainder >= drop {
			displayFrame += drop * ((remainder - drop) / framesPerMinute)
		}
	} else {
		displayFrame %= framesPer24Hours
	}
	tc := Timecode{
		Hours:   int(displayFrame / (nominal * 3600)),
		Minutes: int((displayFrame / (nominal * 60)) % 60),
		Seconds: int((displayFrame / nominal) % 60),
		Frames:  int(displayFrame % nominal),
		Rate:    rate,
	}
	if err := tc.Validate(); err != nil {
		return Timecode{}, err
	}
	return tc, nil
}

func ApplyOffset(frame int64, offsetFrames int64) (int64, error) {
	result := frame + offsetFrames
	if result < 0 {
		return 0, errors.New("timecode offset produces a negative frame")
	}
	return result, nil
}

package timecode

import "errors"

func EncodeMTCQuarterFrame(tc Timecode) ([8]byte, error) {
	var out [8]byte
	if err := tc.Validate(); err != nil {
		return out, err
	}
	rateCode, ok := tc.Rate.MTCCode()
	if !ok {
		return out, errors.New("frame rate cannot be represented by MTC quarter-frame messages")
	}
	nibbles := [8]byte{
		byte(tc.Frames) & 0x0f,
		(byte(tc.Frames) >> 4) & 0x01,
		byte(tc.Seconds) & 0x0f,
		(byte(tc.Seconds) >> 4) & 0x03,
		byte(tc.Minutes) & 0x0f,
		(byte(tc.Minutes) >> 4) & 0x03,
		byte(tc.Hours) & 0x0f,
		((byte(tc.Hours) >> 4) & 0x01) | ((rateCode & 0x03) << 1),
	}
	for i := range out {
		out[i] = byte(i<<4) | nibbles[i]
	}
	return out, nil
}

type MTCDecoder struct {
	nibbles   [8]byte
	seen      uint8
	lastPiece int
	direction int
}

func (d *MTCDecoder) Reset() {
	d.seen = 0
	d.lastPiece = -1
	d.direction = 0
}

func (d *MTCDecoder) Push(data byte) (Timecode, bool, error) {
	piece := int((data >> 4) & 0x07)
	nibble := data & 0x0f
	if d.seen == 0 {
		d.lastPiece = piece
		d.direction = 0
	} else {
		forward := (d.lastPiece + 1) & 7
		reverse := (d.lastPiece + 7) & 7
		switch {
		case piece == forward && (d.direction == 0 || d.direction == 1):
			d.direction = 1
		case piece == reverse && (d.direction == 0 || d.direction == -1):
			d.direction = -1
		default:
			d.Reset()
			d.lastPiece = piece
		}
	}
	d.nibbles[piece] = nibble
	d.seen |= 1 << piece
	d.lastPiece = piece
	if d.seen != 0xff {
		return Timecode{}, false, nil
	}
	rate, err := RateFromMTCCode((d.nibbles[7] >> 1) & 0x03)
	if err != nil {
		d.Reset()
		return Timecode{}, false, err
	}
	tc := Timecode{
		Frames:  int(d.nibbles[0] | ((d.nibbles[1] & 0x01) << 4)),
		Seconds: int(d.nibbles[2] | ((d.nibbles[3] & 0x03) << 4)),
		Minutes: int(d.nibbles[4] | ((d.nibbles[5] & 0x03) << 4)),
		Hours:   int(d.nibbles[6] | ((d.nibbles[7] & 0x01) << 4)),
		Rate:    rate,
	}
	if err := tc.Validate(); err != nil {
		d.Reset()
		return Timecode{}, false, err
	}
	d.seen = 0
	return tc, true, nil
}

type LTCFrame struct {
	Timecode   Timecode
	ReceivedAt int64
	UserBits   [8]byte
}

type LTCDecoder interface {
	DecodeLTC([]byte) (LTCFrame, error)
}

type LTCEncoder interface {
	EncodeLTC(Timecode) ([]byte, error)
}

type LTCAdapter interface {
	Decoder() LTCDecoder
	Encoder() LTCEncoder
}

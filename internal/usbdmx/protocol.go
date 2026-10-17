// Package usbdmx implements the pure, side-effect-free wire format used by
// DMXking ultraDMX Micro (0084), an ENTTEC DMX USB Pro v1.44-compatible
// USB-DMX interface. This package performs no serial or lighting I/O.
package usbdmx

import (
	"errors"
	"fmt"
)

const (
	UniverseChannels = 512
	// ENTTEC USB Pro Widget API 1.44: `Output Only Send DMX Packet Request`
	// label 6, containing start code 0x00 and 512 DMX channel levels.
	OutputDMXLabel  byte = 0x06
	StartDelimiter  byte = 0x7e
	EndDelimiter    byte = 0xe7
	DMXStartCode    byte = 0
	PayloadSize          = 1 + UniverseChannels
	FrameSize            = 1 + 1 + 2 + PayloadSize + 1
)

var ErrInvalidDMXLevels = errors.New("usbdmx: expected exactly 512 channel values")

// Levels is one complete DMX universe. Index 0 is DMX channel 1.
// Value 0 is dark/minimum, 255 is maximum (subject to fixture configuration).
type Levels [UniverseChannels]byte

// EncodeDMXFrame builds a full 518-byte USB Pro message (NOT raw serial DMX
// wire data). The interface's microcontroller generates the DMX BREAK and MAB.
// The message does not change active hardware unless a separately reviewed,
// explicitly enabled serial transport sends it.
func EncodeDMXFrame(levels Levels) [FrameSize]byte {
	var packet [FrameSize]byte
	packet[0] = StartDelimiter
	packet[1] = OutputDMXLabel
	packet[2] = byte(PayloadSize & 0xff)
	packet[3] = byte((PayloadSize >> 8) & 0xff)
	packet[4] = DMXStartCode
	copy(packet[5:5+UniverseChannels], levels[:])
	packet[FrameSize-1] = EndDelimiter
	return packet
}

// FrameFromSlice prevents truncated/non-512 Cues or unsafe channel indexing.
// Never silently pad or truncate an input frame.
func FrameFromSlice(values []byte) ([FrameSize]byte, error) {
	if len(values) != UniverseChannels {
		return [FrameSize]byte{}, fmt.Errorf("%w: got %d", ErrInvalidDMXLevels, len(values))
	}
	var levels Levels
	copy(levels[:], values)
	return EncodeDMXFrame(levels), nil
}

// ZeroFrame is a 512-channel blackout request for an output-only adapter.
// Sending a zero frame does NOT prove actual fixture blackout: cables,
// interface health, output mode, and physical lamp state require qualification.
func ZeroFrame() [FrameSize]byte {
	return EncodeDMXFrame(Levels{})
}

// FrameForChannels accepts 1-based DMX channel addresses, including channel
// 512. Unspecified channels are written as zero; it cannot leave stale slots
// active merely because a Cue omitted them.
func FrameForChannels(channels map[int]byte) ([FrameSize]byte, error) {
	var levels Levels
	for address, level := range channels {
		if address < 1 || address > UniverseChannels {
			return [FrameSize]byte{}, fmt.Errorf("usbdmx: channel %d outside 1..512", address)
		}
		levels[address-1] = level
	}
	return EncodeDMXFrame(levels), nil
}

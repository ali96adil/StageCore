package osc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
)

func Encode(address string, args []any) ([]byte, error) {
	if !strings.HasPrefix(address, "/") || strings.ContainsRune(address, '\x00') {
		return nil, errors.New("invalid OSC address")
	}
	var out bytes.Buffer
	writeString(&out, address)
	tags := ","
	for _, arg := range args {
		switch arg.(type) {
		case string:
			tags += "s"
		case float64, float32:
			tags += "f"
		case int, int32, int64:
			tags += "i"
		case bool:
			if arg.(bool) {
				tags += "T"
			} else {
				tags += "F"
			}
		default:
			return nil, errors.New("unsupported OSC argument type")
		}
	}
	writeString(&out, tags)
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			writeString(&out, v)
		case float64:
			_ = binary.Write(&out, binary.BigEndian, math.Float32bits(float32(v)))
		case float32:
			_ = binary.Write(&out, binary.BigEndian, math.Float32bits(v))
		case int:
			_ = binary.Write(&out, binary.BigEndian, int32(v))
		case int32:
			_ = binary.Write(&out, binary.BigEndian, v)
		case int64:
			_ = binary.Write(&out, binary.BigEndian, int32(v))
		case bool:
			// OSC true/false carry no payload.
		}
	}
	return out.Bytes(), nil
}

func writeString(out *bytes.Buffer, s string) {
	out.WriteString(s)
	out.WriteByte(0)
	for out.Len()%4 != 0 {
		out.WriteByte(0)
	}
}

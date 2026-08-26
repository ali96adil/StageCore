package osc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

type Argument struct {
	Type  string `json:"type"`
	Value any    `json:"value,omitempty"`
}

type Message struct {
	Address   string     `json:"address"`
	Arguments []Argument `json:"arguments,omitempty"`
}

func EncodeMessage(m Message) ([]byte, error) {
	if err := validateAddress(m.Address); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	writePaddedString(&out, m.Address)

	var tags strings.Builder
	tags.WriteByte(',')
	for _, arg := range m.Arguments {
		switch arg.Type {
		case "int32":
			tags.WriteByte('i')
		case "float32":
			tags.WriteByte('f')
		case "string":
			tags.WriteByte('s')
		case "bool":
			v, ok := arg.Value.(bool)
			if !ok {
				return nil, fmt.Errorf("bool argument requires bool value")
			}
			if v {
				tags.WriteByte('T')
			} else {
				tags.WriteByte('F')
			}
		default:
			return nil, fmt.Errorf("unsupported OSC argument type %q", arg.Type)
		}
	}
	writePaddedString(&out, tags.String())

	for _, arg := range m.Arguments {
		switch arg.Type {
		case "int32":
			v, err := asInt32(arg.Value)
			if err != nil {
				return nil, err
			}
			if err := binary.Write(&out, binary.BigEndian, v); err != nil {
				return nil, err
			}
		case "float32":
			v, err := asFloat32(arg.Value)
			if err != nil {
				return nil, err
			}
			if err := binary.Write(&out, binary.BigEndian, math.Float32bits(v)); err != nil {
				return nil, err
			}
		case "string":
			v, ok := arg.Value.(string)
			if !ok {
				return nil, fmt.Errorf("string argument requires string value")
			}
			writePaddedString(&out, v)
		case "bool":
			// OSC T/F tags contain no payload bytes.
		}
	}
	return out.Bytes(), nil
}

func DecodeMessage(packet []byte) (Message, error) {
	address, offset, err := readPaddedString(packet, 0)
	if err != nil {
		return Message{}, fmt.Errorf("address: %w", err)
	}
	if err := validateAddress(address); err != nil {
		return Message{}, err
	}

	tags, offset, err := readPaddedString(packet, offset)
	if err != nil {
		return Message{}, fmt.Errorf("type tags: %w", err)
	}
	if tags == "" || tags[0] != ',' {
		return Message{}, errors.New("OSC type tag string must begin with comma")
	}

	msg := Message{Address: address}
	for _, tag := range tags[1:] {
		switch tag {
		case 'i':
			if offset+4 > len(packet) {
				return Message{}, errors.New("truncated int32 argument")
			}
			v := int32(binary.BigEndian.Uint32(packet[offset : offset+4]))
			offset += 4
			msg.Arguments = append(msg.Arguments, Argument{Type: "int32", Value: v})
		case 'f':
			if offset+4 > len(packet) {
				return Message{}, errors.New("truncated float32 argument")
			}
			bits := binary.BigEndian.Uint32(packet[offset : offset+4])
			offset += 4
			msg.Arguments = append(msg.Arguments, Argument{Type: "float32", Value: math.Float32frombits(bits)})
		case 's':
			v, next, err := readPaddedString(packet, offset)
			if err != nil {
				return Message{}, fmt.Errorf("string argument: %w", err)
			}
			offset = next
			msg.Arguments = append(msg.Arguments, Argument{Type: "string", Value: v})
		case 'T':
			msg.Arguments = append(msg.Arguments, Argument{Type: "bool", Value: true})
		case 'F':
			msg.Arguments = append(msg.Arguments, Argument{Type: "bool", Value: false})
		default:
			return Message{}, fmt.Errorf("unsupported OSC type tag %q", tag)
		}
	}
	return msg, nil
}

func validateAddress(address string) error {
	if !strings.HasPrefix(address, "/") {
		return errors.New("OSC address must start with /")
	}
	if strings.ContainsRune(address, '\x00') || strings.ContainsAny(address, " \t\r\n") {
		return errors.New("OSC address contains invalid whitespace or NUL")
	}
	return nil
}

func writePaddedString(out *bytes.Buffer, s string) {
	out.WriteString(s)
	out.WriteByte(0)
	for out.Len()%4 != 0 {
		out.WriteByte(0)
	}
}

func readPaddedString(packet []byte, offset int) (string, int, error) {
	if offset < 0 || offset >= len(packet) {
		return "", offset, errors.New("string starts outside packet")
	}
	end := offset
	for end < len(packet) && packet[end] != 0 {
		end++
	}
	if end == len(packet) {
		return "", offset, errors.New("unterminated string")
	}
	next := end + 1
	for next%4 != 0 {
		next++
	}
	if next > len(packet) {
		return "", offset, errors.New("truncated padded string")
	}
	return string(packet[offset:end]), next, nil
}

func asInt32(v any) (int32, error) {
	switch n := v.(type) {
	case int:
		if int64(n) < math.MinInt32 || int64(n) > math.MaxInt32 {
			return 0, errors.New("int32 argument out of range")
		}
		return int32(n), nil
	case int32:
		return n, nil
	case int64:
		if n < math.MinInt32 || n > math.MaxInt32 {
			return 0, errors.New("int32 argument out of range")
		}
		return int32(n), nil
	case float64:
		if n != math.Trunc(n) || n < math.MinInt32 || n > math.MaxInt32 {
			return 0, errors.New("int32 argument requires integral in-range value")
		}
		return int32(n), nil
	default:
		return 0, fmt.Errorf("int32 argument has unsupported value type %T", v)
	}
}

func asFloat32(v any) (float32, error) {
	switch n := v.(type) {
	case float32:
		return n, nil
	case float64:
		return float32(n), nil
	case int:
		return float32(n), nil
	case int32:
		return float32(n), nil
	default:
		return 0, fmt.Errorf("float32 argument has unsupported value type %T", v)
	}
}

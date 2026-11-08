package stageaudio

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Bluetooth control is intentionally confined to devices already paired via
// BlueZ. No unrestricted pairing agent, shell or trust escalation is offered.
// Pair once locally with the official BlueZ pairing workflow; StageCore's
// authenticated scanning/pairing UI is a separate security-reviewed phase.

var bluetoothAddress = regexp.MustCompile(`^(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

type BluetoothDevice struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
	Paired    bool   `json:"paired"`
	AudioSink bool   `json:"audio_sink"`
}

type bluetoothRunner interface {
	Run(context.Context, ...string) (string, error)
}

type nativeBluetoothRunner struct{}

func (nativeBluetoothRunner) Run(ctx context.Context, args ...string) (string, error) {
	// Bluetoothctl is a host-local D-Bus client; args are fixed or validated
	// MAC addresses and are NOT interpreted by a shell.
	cmd := exec.CommandContext(ctx, "bluetoothctl", args...)
	output, err := cmd.CombinedOutput()
	if len(output) > 32<<10 {
		return "", errors.New("bluetoothctl response exceeds limit")
	}
	return string(output), err
}

type BluetoothService struct {
	runner bluetoothRunner
}

func NewBluetoothService() *BluetoothService {
	return &BluetoothService{runner: nativeBluetoothRunner{}}
}

func bluetoothTimed(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

func (b *BluetoothService) paired(ctx context.Context) ([]BluetoothDevice, error) {
	c, cancel := bluetoothTimed(ctx, 5*time.Second)
	defer cancel()
	output, err := b.runner.Run(c, "devices", "Paired")
	if err != nil {
		return nil, fmt.Errorf("%w: Bluetooth discovery unavailable: %v", ErrBackend, err)
	}
	items := make([]BluetoothDevice, 0)
	known := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// BlueZ prints lines in the form: Device AA:BB:... Friendly name
		parts := strings.Fields(line)
		if len(parts) < 3 || parts[0] != "Device" || !bluetoothAddress.MatchString(parts[1]) {
			continue
		}
		address := strings.ToUpper(parts[1])
		if known[address] {
			continue
		}
		known[address] = true
		label := strings.Join(parts[2:], " ")
		if len(label) > 120 {
			label = label[:120]
		}
		items = append(items, BluetoothDevice{Address: address, Name: label, Paired: true})
	}
	return items, nil
}

func (b *BluetoothService) PairedSpeakers(ctx context.Context) ([]BluetoothDevice, error) {
	devices, err := b.paired(ctx)
	if err != nil {
		return nil, err
	}
	speakers := make([]BluetoothDevice, 0, len(devices))
	for _, device := range devices {
		info, err := b.Info(ctx, device.Address)
		if err != nil {
			continue // device disconnected/vanished; do not imply healthy
		}
		if info.Paired && info.AudioSink {
			speakers = append(speakers, info)
		}
	}
	return speakers, nil
}

func (b *BluetoothService) Info(ctx context.Context, address string) (BluetoothDevice, error) {
	if !bluetoothAddress.MatchString(address) {
		return BluetoothDevice{}, ErrInvalidCommand
	}
	c, cancel := bluetoothTimed(ctx, 5*time.Second)
	defer cancel()
	output, err := b.runner.Run(c, "info", strings.ToUpper(address))
	if err != nil {
		return BluetoothDevice{}, fmt.Errorf("%w: Bluetooth device unavailable: %v", ErrBackend, err)
	}
	result := BluetoothDevice{Address: strings.ToUpper(address)}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Name:"):
			result.Name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
		case strings.HasPrefix(line, "Alias:") && result.Name == "":
			result.Name = strings.TrimSpace(strings.TrimPrefix(line, "Alias:"))
		case strings.EqualFold(line, "Paired: yes"):
			result.Paired = true
		case strings.EqualFold(line, "Connected: yes"):
			result.Connected = true
		case strings.Contains(strings.ToLower(line), "0000110b-0000-1000-8000-00805f9b34fb") ||
			strings.Contains(strings.ToLower(line), "audio sink"):
			result.AudioSink = true
		}
	}
	if !result.Paired {
		return BluetoothDevice{}, fmt.Errorf("%w: Bluetooth speaker is not paired", ErrInvalidCommand)
	}
	return result, nil
}

// ConnectPairedSpeaker can connect an existing A2DP speaker, but does not
// change any audio route; SELECT_OUTPUT must be separately confirmed.
func (b *BluetoothService) ConnectPairedSpeaker(ctx context.Context, address string) (BluetoothDevice, error) {
	info, err := b.Info(ctx, address)
	if err != nil {
		return BluetoothDevice{}, err
	}
	if !info.AudioSink {
		return BluetoothDevice{}, fmt.Errorf("%w: device does not advertise A2DP Audio Sink", ErrInvalidCommand)
	}
	if info.Connected {
		return info, nil
	}
	c, cancel := bluetoothTimed(ctx, 18*time.Second)
	defer cancel()
	if _, err := b.runner.Run(c, "connect", info.Address, "a2dp-sink"); err != nil {
		return BluetoothDevice{}, fmt.Errorf("%w: Bluetooth A2DP connect failed: %v", ErrBackend, err)
	}
	confirmed, err := b.Info(ctx, info.Address)
	if err != nil {
		return BluetoothDevice{}, err
	}
	if !confirmed.Connected {
		return BluetoothDevice{}, fmt.Errorf("%w: Bluetooth connection not confirmed", ErrBackend)
	}
	return confirmed, nil
}

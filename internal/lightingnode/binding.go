package lightingnode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const ProfileID = "stagecore.esp32-dmx-lighting-node"

var logicalAliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type ProjectBinding struct {
	RevisionID    string            `json:"revision_id,omitempty"`
	DeviceID      string            `json:"device_id"`
	ProfileID     string            `json:"profile_id"`
	Configuration Configuration     `json:"configuration"`
	Aliases       map[string]string `json:"aliases"`
	UpdatedBy     string            `json:"updated_by,omitempty"`
}

type ResolvedChannel struct {
	Alias         string      `json:"alias"`
	DeviceID      string      `json:"device_id"`
	ProfileID     string      `json:"profile_id"`
	ChannelKey    string      `json:"channel_key"`
	ChannelNumber int         `json:"channel_number"`
	DisplayName   string      `json:"display_name"`
	Kind          ChannelKind `json:"kind"`
	PhysicalZone  string      `json:"physical_zone,omitempty"`
	MinimumLevel  float64     `json:"minimum_level"`
	MaximumLevel  float64     `json:"maximum_level"`
	Inverted      bool        `json:"inverted"`
}

func ValidateProjectBinding(binding ProjectBinding) error {
	binding.DeviceID = strings.TrimSpace(binding.DeviceID)
	binding.ProfileID = strings.TrimSpace(binding.ProfileID)
	if binding.DeviceID == "" {
		return fmt.Errorf("lighting binding requires device_id")
	}
	if binding.ProfileID == "" {
		binding.ProfileID = ProfileID
	}
	if binding.ProfileID != ProfileID {
		return fmt.Errorf("lighting binding profile_id must be %q", ProfileID)
	}
	if err := ValidateConfiguration(binding.Configuration); err != nil {
		return err
	}
	index := ChannelIndex(binding.Configuration)
	usedChannels := make(map[string]string)
	for alias, channelKey := range binding.Aliases {
		alias = strings.TrimSpace(alias)
		channelKey = strings.TrimSpace(channelKey)
		if !logicalAliasPattern.MatchString(alias) {
			return fmt.Errorf("invalid lighting logical alias %q", alias)
		}
		channel, ok := index[channelKey]
		if !ok {
			return fmt.Errorf("lighting alias %q references unknown channel %q", alias, channelKey)
		}
		if !channel.Enabled || channel.Kind == ChannelUnused {
			return fmt.Errorf("lighting alias %q references disabled channel %q", alias, channelKey)
		}
		if previous, exists := usedChannels[channelKey]; exists {
			return fmt.Errorf("lighting channel %q is mapped by both %q and %q", channelKey, previous, alias)
		}
		usedChannels[channelKey] = alias
	}
	return nil
}

func ValidateProjectBindings(bindings []ProjectBinding) error {
	aliases := make(map[string]string)
	devices := make(map[string]struct{})
	for _, binding := range bindings {
		if err := ValidateProjectBinding(binding); err != nil {
			return err
		}
		deviceID := strings.TrimSpace(binding.DeviceID)
		if _, exists := devices[deviceID]; exists {
			return fmt.Errorf("duplicate lighting binding for device %q", deviceID)
		}
		devices[deviceID] = struct{}{}
		for alias := range binding.Aliases {
			normalized := strings.TrimSpace(alias)
			if previous, exists := aliases[normalized]; exists {
				return fmt.Errorf("lighting alias %q is ambiguous across devices %q and %q", normalized, previous, deviceID)
			}
			aliases[normalized] = deviceID
		}
	}
	return nil
}

func ResolveAlias(bindings []ProjectBinding, alias string) (*ResolvedChannel, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil, fmt.Errorf("lighting alias is required")
	}
	if err := ValidateProjectBindings(bindings); err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		channelKey, ok := binding.Aliases[alias]
		if !ok {
			continue
		}
		channel := ChannelIndex(binding.Configuration)[channelKey]
		return &ResolvedChannel{
			Alias:         alias,
			DeviceID:      binding.DeviceID,
			ProfileID:     binding.ProfileID,
			ChannelKey:    channel.ChannelKey,
			ChannelNumber: channel.ChannelNumber,
			DisplayName:   channel.DisplayName,
			Kind:          channel.Kind,
			PhysicalZone:  channel.PhysicalZone,
			MinimumLevel:  channel.MinimumLevel,
			MaximumLevel:  channel.MaximumLevel,
			Inverted:      channel.Inverted,
		}, nil
	}
	return nil, fmt.Errorf("lighting alias %q is not bound", alias)
}

func CanonicalProjectBinding(binding ProjectBinding) ([]byte, error) {
	if binding.ProfileID == "" {
		binding.ProfileID = ProfileID
	}
	if binding.Configuration.SchemaVersion == 0 {
		binding.Configuration.SchemaVersion = SchemaVersion1
	}
	if err := ValidateProjectBinding(binding); err != nil {
		return nil, err
	}
	binding.DeviceID = strings.TrimSpace(binding.DeviceID)
	binding.ProfileID = strings.TrimSpace(binding.ProfileID)
	binding.RevisionID = strings.TrimSpace(binding.RevisionID)
	binding.UpdatedBy = strings.TrimSpace(binding.UpdatedBy)

	aliases := make(map[string]string, len(binding.Aliases))
	keys := make([]string, 0, len(binding.Aliases))
	for alias, channelKey := range binding.Aliases {
		alias = strings.TrimSpace(alias)
		aliases[alias] = strings.TrimSpace(channelKey)
		keys = append(keys, alias)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	for _, key := range keys {
		ordered[key] = aliases[key]
	}
	binding.Aliases = ordered
	return json.Marshal(binding)
}

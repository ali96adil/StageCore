package deviceexperience

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/contracts"
)

func normalizeCapabilities(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeJSON(value json.RawMessage, fallback string) json.RawMessage {
	if len(value) == 0 || !json.Valid(value) {
		return json.RawMessage(fallback)
	}
	return value
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func validDeviceKind(value DeviceKind) bool {
	switch value {
	case DeviceTabletPlayer, DeviceStageDisplay, DeviceRenderNode, DeviceGeneric:
		return true
	default:
		return false
	}
}

func validConnectionState(value ConnectionState) bool {
	switch value {
	case ConnectionOnline, ConnectionOffline, ConnectionStale, ConnectionRevoked:
		return true
	default:
		return false
	}
}

func validReadiness(value Readiness) bool {
	switch value {
	case ReadinessReady, ReadinessWarning, ReadinessAdvisory, ReadinessBlocker, ReadinessUnknown:
		return true
	default:
		return false
	}
}

func validDisplayMode(value DisplayMode) bool {
	switch value {
	case DisplayIdle, DisplayMessage, DisplayCountdown, DisplayAlert, DisplayBlackout:
		return true
	default:
		return false
	}
}

func displayCapability(mode DisplayMode) string {
	switch mode {
	case DisplayIdle:
		return "display.clear"
	case DisplayMessage:
		return "display.message.show"
	case DisplayCountdown:
		return "display.countdown.show"
	case DisplayAlert:
		return "display.alert.show"
	case DisplayBlackout:
		return "display.blackout"
	default:
		return ""
	}
}

func validSourceClass(value SourceClass) bool {
	switch value {
	case SourceLocalCamera, SourceUSBCapture, SourceNetworkStream:
		return true
	default:
		return false
	}
}

func validTargetKind(value string) bool {
	switch value {
	case "HUB", "COMPANION", "STAGE_DEVICE", "LIVE_SOURCE", "ENDPOINT":
		return true
	default:
		return false
	}
}

func validReachability(value Reachability) bool {
	switch value {
	case Reachable, Unreachable, ReachUnknown:
		return true
	default:
		return false
	}
}

func terminalCommandStatus(status contracts.CommandStatus) bool {
	switch status {
	case contracts.CommandRejected, contracts.CommandCompleted, contracts.CommandFailed,
		contracts.CommandTimedOut, contracts.CommandCancelled:
		return true
	default:
		return false
	}
}

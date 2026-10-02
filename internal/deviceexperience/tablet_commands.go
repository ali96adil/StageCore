package deviceexperience

// Tablet Controller extends the Stage Device command vocabulary without
// introducing a second transport. All commands continue through stagecore.device/1.
const (
	CommandTabletPrepare       = "TABLET_PREPARE"
	CommandTabletPlay          = "TABLET_PLAY"
	CommandTabletPause         = "TABLET_PAUSE"
	CommandTabletStop          = "TABLET_STOP"
	CommandTabletBlackout      = "TABLET_BLACKOUT"
	CommandTabletBlackoutClear = "TABLET_BLACKOUT_CLEAR"
	CommandTabletOverlayPlay   = "TABLET_OVERLAY_PLAY"
	CommandTabletOverlayClear  = "TABLET_OVERLAY_CLEAR"
	CommandTabletLiveShow      = "TABLET_LIVE_SHOW"
	CommandTabletLiveHide      = "TABLET_LIVE_HIDE"
	CommandTabletBrightnessSet    = "TABLET_BRIGHTNESS_SET"
	CommandTabletShowModeSet      = "TABLET_SHOW_MODE_SET"
	CommandTabletVideoScaleSet    = "TABLET_VIDEO_SCALE_SET"
	CommandTabletOrientationSet   = "TABLET_ORIENTATION_SET"
	CommandTabletLiveRotationSet  = "TABLET_LIVE_ROTATION_SET"
)

const (
	CapabilityTabletPrepare       = "tablet.media.prepare"
	CapabilityTabletPlay          = "tablet.media.play"
	CapabilityTabletPause         = "tablet.media.pause"
	CapabilityTabletStop          = "tablet.media.stop"
	CapabilityTabletBlackout      = "tablet.media.blackout"
	CapabilityTabletBlackoutClear = "tablet.media.blackout.clear"
	CapabilityTabletOverlayPlay   = "tablet.media.overlay.play"
	CapabilityTabletOverlayClear  = "tablet.media.overlay.clear"
	CapabilityTabletLiveShow      = "tablet.media.live.show"
	CapabilityTabletLiveHide      = "tablet.media.live.hide"
	CapabilityTabletBrightnessSet   = "tablet.settings.brightness.set"
	CapabilityTabletShowModeSet     = "tablet.settings.show_mode.set"
	CapabilityTabletVideoScaleSet   = "tablet.settings.video_scale.set"
	CapabilityTabletOrientationSet  = "tablet.settings.orientation.set"
	CapabilityTabletLiveRotationSet = "tablet.settings.live_rotation.set"
)

func init() {
	commandCapability[CommandTabletPrepare] = CapabilityTabletPrepare
	commandCapability[CommandTabletPlay] = CapabilityTabletPlay
	commandCapability[CommandTabletPause] = CapabilityTabletPause
	commandCapability[CommandTabletStop] = CapabilityTabletStop
	commandCapability[CommandTabletBlackout] = CapabilityTabletBlackout
	commandCapability[CommandTabletBlackoutClear] = CapabilityTabletBlackoutClear
	commandCapability[CommandTabletOverlayPlay] = CapabilityTabletOverlayPlay
	commandCapability[CommandTabletOverlayClear] = CapabilityTabletOverlayClear
	commandCapability[CommandTabletLiveShow] = CapabilityTabletLiveShow
	commandCapability[CommandTabletLiveHide] = CapabilityTabletLiveHide
	commandCapability[CommandTabletBrightnessSet] = CapabilityTabletBrightnessSet
	commandCapability[CommandTabletShowModeSet] = CapabilityTabletShowModeSet
	commandCapability[CommandTabletVideoScaleSet] = CapabilityTabletVideoScaleSet
	commandCapability[CommandTabletOrientationSet] = CapabilityTabletOrientationSet
	commandCapability[CommandTabletLiveRotationSet] = CapabilityTabletLiveRotationSet
}

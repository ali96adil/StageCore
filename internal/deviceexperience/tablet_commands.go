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
	CommandTabletSelectMedia   = "TABLET_SELECT_MEDIA"
	CommandTabletOverlayPlay   = "TABLET_OVERLAY_PLAY"
	CommandTabletOverlayClear  = "TABLET_OVERLAY_CLEAR"
	CommandTabletLiveShow      = "TABLET_LIVE_SHOW"
	CommandTabletLiveHide      = "TABLET_LIVE_HIDE"
)

const (
	CapabilityTabletPrepare       = "tablet.media.prepare"
	CapabilityTabletPlay          = "tablet.media.play"
	CapabilityTabletPause         = "tablet.media.pause"
	CapabilityTabletStop          = "tablet.media.stop"
	CapabilityTabletBlackout      = "tablet.media.blackout"
	CapabilityTabletBlackoutClear = "tablet.media.blackout.clear"
	CapabilityTabletSelectMedia   = "tablet.media.select"
	CapabilityTabletOverlayPlay   = "tablet.media.overlay.play"
	CapabilityTabletOverlayClear  = "tablet.media.overlay.clear"
	CapabilityTabletLiveShow      = "tablet.media.live.show"
	CapabilityTabletLiveHide      = "tablet.media.live.hide"
)

func init() {
	commandCapability[CommandTabletPrepare] = CapabilityTabletPrepare
	commandCapability[CommandTabletPlay] = CapabilityTabletPlay
	commandCapability[CommandTabletPause] = CapabilityTabletPause
	commandCapability[CommandTabletStop] = CapabilityTabletStop
	commandCapability[CommandTabletBlackout] = CapabilityTabletBlackout
	commandCapability[CommandTabletBlackoutClear] = CapabilityTabletBlackoutClear
	commandCapability[CommandTabletSelectMedia] = CapabilityTabletSelectMedia
	commandCapability[CommandTabletOverlayPlay] = CapabilityTabletOverlayPlay
	commandCapability[CommandTabletOverlayClear] = CapabilityTabletOverlayClear
	commandCapability[CommandTabletLiveShow] = CapabilityTabletLiveShow
	commandCapability[CommandTabletLiveHide] = CapabilityTabletLiveHide
}

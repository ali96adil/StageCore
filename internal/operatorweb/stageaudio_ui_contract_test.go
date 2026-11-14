package operatorweb

import (
	"strings"
	"testing"
)

func TestStageAudioPhoneUIEmbedsOfflineTwoDeckAndOutputControls(t *testing.T) {
	index := string(mustReadOperatorContractFile(t, "static/index.html"))
	js := string(mustReadOperatorContractFile(t, "static/stageaudio.js"))
	css := string(mustReadOperatorContractFile(t, "static/app.css"))
	for _, marker := range []string{`id="stageAudioNav"`, `src="/stageaudio.js"`} {
		if !strings.Contains(index, marker) { t.Fatalf("missing Audio workspace navigation %q",marker) }
	}
	for _, marker := range []string{
		`/api/v1/system/audio/status`, `/api/v1/system/audio/outputs`,
		`/api/v1/system/audio/bluetooth`, `/api/v1/system/audio/media`,
		`/api/v1/system/audio/command`, `scAudioDeckHtml("A")`,
		`scAudioDeckHtml("B")`, `SELECT_OUTPUT`, `BT_CONNECT`, `MASTER_VOLUME`,
		`state.page === "audio"`, `s.active_session === "SHOW"`,
		`canRuntime()`,
	} {
		if !strings.Contains(js, marker) { t.Errorf("missing player contract %q",marker) }
	}
	for _, forbidden := range []string{`new Audio(`,`AudioContext(`,`http://`,`https://`,`input-ipc-server`,`bluetoothctl`} {
		if strings.Contains(js,forbidden) { t.Errorf("browser may not play or directly control sound %q",forbidden) }
	}
	if !strings.Contains(css, ".sc-audio-decks") { t.Fatal("mobile audio player layout missing") }
}

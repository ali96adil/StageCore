{
  "patcher": {
    "fileversion": 1,
    "appversion": {
      "major": 9,
      "minor": 0,
      "revision": 0,
      "architecture": "x64",
      "modernui": 1
    },
    "classnamespace": "box",
    "rect": [
      80,
      80,
      920,
      565
    ],
    "bglocked": 0,
    "openinpresentation": 0,
    "default_fontsize": 12,
    "default_fontface": 0,
    "default_fontname": "Arial",
    "boxes": [
      {
        "box": {
          "id": "header",
          "maxclass": "comment",
          "text": "STAGECORE 5K LAB ONLY - Max for Live observation harness. No live control, transport, or trustworthy playback proof.",
          "patching_rect": [
            30,
            18,
            840,
            35
          ]
        }
      },
      {
        "box": {
          "id": "note",
          "maxclass": "comment",
          "text": "Do not install during a show. No actual .amxd build included. Load as a patch inside an M4L editor to test Track 0 playing_slot_index.",
          "patching_rect": [
            30,
            52,
            830,
            36
          ]
        }
      },
      {
        "box": {
          "id": "m4l",
          "maxclass": "newobj",
          "text": "live.thisdevice",
          "patching_rect": [
            40,
            120,
            100,
            22
          ]
        }
      },
      {
        "box": {
          "id": "defer",
          "maxclass": "newobj",
          "text": "deferlow",
          "patching_rect": [
            40,
            164,
            66,
            22
          ]
        }
      },
      {
        "box": {
          "id": "path",
          "maxclass": "newobj",
          "text": "live.path live_set tracks 0",
          "patching_rect": [
            40,
            205,
            220,
            22
          ]
        }
      },
      {
        "box": {
          "id": "observer",
          "maxclass": "newobj",
          "text": "live.observer playing_slot_index",
          "patching_rect": [
            40,
            251,
            205,
            22
          ]
        }
      },
      {
        "box": {
          "id": "route",
          "maxclass": "newobj",
          "text": "route playing_slot_index",
          "patching_rect": [
            40,
            293,
            208,
            22
          ]
        }
      },
      {
        "box": {
          "id": "pre",
          "maxclass": "newobj",
          "text": "prepend stagecore_live_slot 0",
          "patching_rect": [
            40,
            338,
            230,
            22
          ]
        }
      },
      {
        "box": {
          "id": "manual",
          "maxclass": "comment",
          "text": "Manual lab arm metadata (one JSON string) enters at the inlet; no automatic cue arming.",
          "patching_rect": [
            382,
            112,
            450,
            40
          ]
        }
      },
      {
        "box": {
          "id": "arm",
          "maxclass": "inlet",
          "patching_rect": [
            394,
            155,
            28,
            28
          ]
        }
      },
      {
        "box": {
          "id": "start",
          "maxclass": "message",
          "text": "script start",
          "patching_rect": [
            390,
            234,
            96,
            22
          ]
        }
      },
      {
        "box": {
          "id": "stop",
          "maxclass": "message",
          "text": "script stop",
          "patching_rect": [
            505,
            234,
            90,
            22
          ]
        }
      },
      {
        "box": {
          "id": "node",
          "maxclass": "newobj",
          "text": "node.script max_receiver_bridge.js @autostart 0",
          "patching_rect": [
            390,
            302,
            430,
            24
          ]
        }
      },
      {
        "box": {
          "id": "print",
          "maxclass": "newobj",
          "text": "print stagecore_ack_lab",
          "patching_rect": [
            390,
            356,
            185,
            22
          ]
        }
      },
      {
        "box": {
          "id": "warning",
          "maxclass": "comment",
          "text": "ACK outputs are BASE64 fixtures ONLY. Do not connect this outlet to udpsend, OSC GO, audio, light, or an armed production StageCore input.",
          "patching_rect": [
            390,
            397,
            448,
            72
          ]
        }
      }
    ],
    "lines": [
      {
        "patchline": {
          "source": [
            "m4l",
            0
          ],
          "destination": [
            "defer",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "defer",
            0
          ],
          "destination": [
            "path",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "path",
            0
          ],
          "destination": [
            "observer",
            1
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "observer",
            0
          ],
          "destination": [
            "route",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "route",
            0
          ],
          "destination": [
            "pre",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "route",
            1
          ],
          "destination": [
            "pre",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "pre",
            0
          ],
          "destination": [
            "node",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "arm",
            0
          ],
          "destination": [
            "node",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "start",
            0
          ],
          "destination": [
            "node",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "stop",
            0
          ],
          "destination": [
            "node",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "node",
            0
          ],
          "destination": [
            "print",
            0
          ]
        }
      }
    ]
  }
}
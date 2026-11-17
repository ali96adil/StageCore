{
  "patcher": {
    "fileversion": 1,
    "rect": [
      0,
      0,
      540,
      370
    ],
    "boxes": [
      {
        "box": {
          "id": "poll",
          "maxclass": "newobj",
          "patching_rect": [
            35,
            100,
            100,
            22
          ],
          "text": "qmetro 125"
        }
      },
      {
        "box": {
          "id": "toggle",
          "maxclass": "toggle",
          "patching_rect": [
            35,
            65,
            22,
            22
          ]
        }
      },
      {
        "box": {
          "id": "msg",
          "maxclass": "message",
          "patching_rect": [
            35,
            134,
            43,
            22
          ],
          "text": "poll"
        }
      },
      {
        "box": {
          "id": "start",
          "maxclass": "message",
          "patching_rect": [
            35,
            166,
            90,
            22
          ],
          "text": "script start"
        }
      },
      {
        "box": {
          "id": "node",
          "maxclass": "newobj",
          "patching_rect": [
            35,
            215,
            300,
            22
          ],
          "text": "node.script max_live_receiver.js @autostart 0"
        }
      },
      {
        "box": {
          "id": "defer",
          "maxclass": "newobj",
          "patching_rect": [
            35,
            255,
            74,
            22
          ],
          "text": "deferlow"
        }
      },
      {
        "box": {
          "id": "js",
          "maxclass": "newobj",
          "patching_rect": [
            35,
            295,
            190,
            22
          ],
          "text": "js stagecore_live_track.js"
        }
      },
      {
        "box": {
          "id": "title",
          "maxclass": "comment",
          "patching_rect": [
            35,
            20,
            430,
            22
          ],
          "text": "StageCore Live receiver \u2014 place on the dedicated test track"
        }
      }
    ],
    "lines": [
      {
        "patchline": {
          "source": [
            "toggle",
            0
          ],
          "destination": [
            "poll",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "poll",
            0
          ],
          "destination": [
            "msg",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "msg",
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
            "node",
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
            "js",
            0
          ]
        }
      },
      {
        "patchline": {
          "source": [
            "js",
            0
          ],
          "destination": [
            "node",
            0
          ]
        }
      }
    ]
  }
}

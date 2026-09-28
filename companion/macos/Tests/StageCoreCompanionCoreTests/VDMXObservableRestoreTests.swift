import Foundation
import Testing
@testable import StageCoreCompanionCore

private func node(
    path: String,
    type: String,
    value: [JSONValue],
    access: Int = 3,
    range: [JSONValue]? = nil
) -> JSONValue {
    var object: [String: JSONValue] = [
        "FULL_PATH": .string(path),
        "TYPE": .string(type),
        "VALUE": .array(value),
        "ACCESS": .int(access),
    ]
    if let range {
        object["RANGE"] = .array(range)
    }
    return .object(object)
}

private func numericRange(_ min: Double, _ max: Double) -> JSONValue {
    .object([
        "MIN": .double(min),
        "MAX": .double(max),
    ])
}

private func namespace(_ children: [String: JSONValue]) -> [String: JSONValue] {
    [
        "FULL_PATH": .string("/"),
        "CONTENTS": .object(children),
    ]
}

@Test("continuous float control is restorable when live metadata is compatible")
func floatControlIsRestorable() {
    let captured = namespace([
        "opacity": node(
            path: "/opacity",
            type: "f",
            value: [.double(0.75)],
            range: [numericRange(0, 1)]
        )
    ])
    let live = namespace([
        "opacity": node(
            path: "/opacity",
            type: "f",
            value: [.double(0.25)],
            range: [numericRange(0, 1)]
        )
    ])

    let preview = VDMXObservableRestorePlanner.preview(
        capturedNamespace: captured,
        liveNamespace: live
    )

    #expect(preview.restorableCount == 1)
    #expect(preview.entries.first?.path == "/opacity")
    #expect(preview.entries.first?.classification == .restorable)
}

@Test("already matching continuous control is not rewritten")
func matchingControlIsAlreadyMatching() {
    let captured = namespace([
        "opacity": node(
            path: "/opacity",
            type: "f",
            value: [.double(0.5)],
            range: [numericRange(0, 1)]
        )
    ])

    let preview = VDMXObservableRestorePlanner.preview(
        capturedNamespace: captured,
        liveNamespace: captured
    )

    #expect(preview.alreadyMatchingCount == 1)
    #expect(preview.restorableCount == 0)
}

@Test("missing, read only, type mismatch and range mismatch remain non-restorable")
func invalidLiveControlsFailClosed() {
    let captured = namespace([
        "missing": node(path: "/missing", type: "f", value: [.double(0.5)], range: [numericRange(0, 1)]),
        "read": node(path: "/read", type: "f", value: [.double(0.5)], range: [numericRange(0, 1)]),
        "type": node(path: "/type", type: "f", value: [.double(0.5)], range: [numericRange(0, 1)]),
        "range": node(path: "/range", type: "f", value: [.double(0.9)], range: [numericRange(0, 1)]),
    ])
    let live = namespace([
        "read": node(path: "/read", type: "f", value: [.double(0.25)], access: 1, range: [numericRange(0, 1)]),
        "type": node(path: "/type", type: "i", value: [.int(0)], range: [numericRange(0, 1)]),
        "range": node(path: "/range", type: "f", value: [.double(0.25)], range: [numericRange(0, 0.5)]),
    ])

    let preview = VDMXObservableRestorePlanner.preview(
        capturedNamespace: captured,
        liveNamespace: live
    )

    #expect(preview.missingCount == 1)
    #expect(preview.readOnlyCount == 1)
    #expect(preview.incompatibleCount == 2)
    #expect(preview.restorableCount == 0)
}

@Test("numeric control without current live range is incompatible")
func numericControlWithoutRangeFailsClosed() {
    let captured = namespace([
        "opacity": node(
            path: "/opacity",
            type: "f",
            value: [.double(0.75)],
            range: [numericRange(0, 1)]
        )
    ])
    let live = namespace([
        "opacity": node(
            path: "/opacity",
            type: "f",
            value: [.double(0.25)]
        )
    ])

    let preview = VDMXObservableRestorePlanner.preview(
        capturedNamespace: captured,
        liveNamespace: live
    )

    #expect(preview.incompatibleCount == 1)
    #expect(preview.restorableCount == 0)
}

@Test("StageCore GO style integer button is unsafe by default even when values match")
func integerButtonIsUnsafeByDefault() {
    let path = "/OSCQUERY/Control Surface/StageCore_Test"
    let captured = namespace([
        "button": node(
            path: path,
            type: "i",
            value: [.int(1)],
            range: [numericRange(0, 1)]
        )
    ])

    let preview = VDMXObservableRestorePlanner.preview(
        capturedNamespace: captured,
        liveNamespace: captured
    )

    #expect(preview.unsafeCount == 1)
    #expect(preview.alreadyMatchingCount == 0)
    #expect(preview.entries.first?.path == path)
}

@Test("explicit exact-path allowlist can permit a validated discrete control")
func allowlistedIntegerCanBeRestorable() {
    let path = "/safe/discrete"
    let captured = namespace([
        "button": node(
            path: path,
            type: "i",
            value: [.int(1)],
            range: [numericRange(0, 1)]
        )
    ])
    let live = namespace([
        "button": node(
            path: path,
            type: "i",
            value: [.int(0)],
            range: [numericRange(0, 1)]
        )
    ])

    let preview = VDMXObservableRestorePlanner.preview(
        capturedNamespace: captured,
        liveNamespace: live,
        explicitUnsafeControlAllowlist: [path]
    )

    #expect(preview.restorableCount == 1)
    #expect(preview.unsafeCount == 0)
}

@Test("nested OSCQuery namespace is matched by exact FULL_PATH")
func nestedNamespaceUsesExactPaths() {
    let captured = namespace([
        "group": .object([
            "FULL_PATH": .string("/group"),
            "CONTENTS": .object([
                "gain": node(
                    path: "/group/gain",
                    type: "f",
                    value: [.double(0.8)],
                    range: [numericRange(0, 1)]
                )
            ]),
        ])
    ])
    let live = namespace([
        "group": .object([
            "FULL_PATH": .string("/group"),
            "CONTENTS": .object([
                "gain": node(
                    path: "/group/gain",
                    type: "f",
                    value: [.double(0.2)],
                    range: [numericRange(0, 1)]
                )
            ]),
        ])
    ])

    let preview = VDMXObservableRestorePlanner.preview(
        capturedNamespace: captured,
        liveNamespace: live
    )

    #expect(preview.entries.count == 1)
    #expect(preview.entries.first?.path == "/group/gain")
    #expect(preview.entries.first?.classification == .restorable)
}

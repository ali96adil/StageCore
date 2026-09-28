import Foundation

public enum VDMXObservableRestoreClassification: String, Sendable, Equatable {
    case restorable = "RESTORABLE"
    case alreadyMatching = "ALREADY_MATCHING"
    case missing = "MISSING"
    case incompatible = "INCOMPATIBLE"
    case readOnly = "READ_ONLY"
    case unsafe = "UNSAFE"
}

public struct VDMXObservableRestoreEntry: Sendable, Equatable {
    public let path: String
    public let classification: VDMXObservableRestoreClassification
    public let reason: String
    public let capturedValue: JSONValue?
    public let liveValue: JSONValue?
}

public struct VDMXObservableRestorePreview: Sendable, Equatable {
    public let entries: [VDMXObservableRestoreEntry]

    public var restorableCount: Int { count(.restorable) }
    public var alreadyMatchingCount: Int { count(.alreadyMatching) }
    public var missingCount: Int { count(.missing) }
    public var incompatibleCount: Int { count(.incompatible) }
    public var readOnlyCount: Int { count(.readOnly) }
    public var unsafeCount: Int { count(.unsafe) }

    private func count(_ classification: VDMXObservableRestoreClassification) -> Int {
        entries.lazy.filter { $0.classification == classification }.count
    }
}

public enum VDMXObservableRestorePlanner {
    public static func preview(
        capturedNamespace: [String: JSONValue],
        liveNamespace: [String: JSONValue],
        explicitUnsafeControlAllowlist: Set<String> = []
    ) -> VDMXObservableRestorePreview {
        let captured = flatten(capturedNamespace)
        let live = flatten(liveNamespace)

        let entries = captured.keys.sorted().compactMap { path -> VDMXObservableRestoreEntry? in
            guard let capturedNode = captured[path],
                  capturedNode["VALUE"] != nil
            else {
                return nil
            }

            guard let liveNode = live[path] else {
                return entry(
                    path: path,
                    classification: .missing,
                    reason: "captured OSC path is not present in the live namespace",
                    capturedNode: capturedNode,
                    liveNode: nil
                )
            }

            guard let capturedType = string(capturedNode["TYPE"]),
                  let liveType = string(liveNode["TYPE"]),
                  capturedType == liveType
            else {
                return entry(
                    path: path,
                    classification: .incompatible,
                    reason: "captured and live OSC types do not match exactly",
                    capturedNode: capturedNode,
                    liveNode: liveNode
                )
            }

            guard isWritable(liveNode) else {
                return entry(
                    path: path,
                    classification: .readOnly,
                    reason: "live OSCQuery ACCESS does not permit writes",
                    capturedNode: capturedNode,
                    liveNode: liveNode
                )
            }

            guard compatibleValue(capturedNode["VALUE"], withType: liveType),
                  compatibleValue(liveNode["VALUE"], withType: liveType)
            else {
                return entry(
                    path: path,
                    classification: .incompatible,
                    reason: "OSC VALUE does not match the declared live TYPE",
                    capturedNode: capturedNode,
                    liveNode: liveNode
                )
            }

            guard capturedValueWithinLiveRange(
                capturedNode["VALUE"],
                liveRange: liveNode["RANGE"]
            ) else {
                return entry(
                    path: path,
                    classification: .incompatible,
                    reason: "captured value falls outside the current live RANGE",
                    capturedNode: capturedNode,
                    liveNode: liveNode
                )
            }

            if !explicitUnsafeControlAllowlist.contains(path),
               !defaultStatefulType(liveType) {
                return entry(
                    path: path,
                    classification: .unsafe,
                    reason: "discrete, integer, boolean or string controls are excluded from automatic restore by default",
                    capturedNode: capturedNode,
                    liveNode: liveNode
                )
            }

            if capturedNode["VALUE"] == liveNode["VALUE"] {
                return entry(
                    path: path,
                    classification: .alreadyMatching,
                    reason: "live value already matches the captured state",
                    capturedNode: capturedNode,
                    liveNode: liveNode
                )
            }

            return entry(
                path: path,
                classification: .restorable,
                reason: explicitUnsafeControlAllowlist.contains(path)
                    ? "exact path is explicitly allowlisted and passed live type/access/range checks"
                    : "stateful numeric control passed live type/access/range checks",
                capturedNode: capturedNode,
                liveNode: liveNode
            )
        }

        return VDMXObservableRestorePreview(entries: entries)
    }

    private static func entry(
        path: String,
        classification: VDMXObservableRestoreClassification,
        reason: String,
        capturedNode: [String: JSONValue],
        liveNode: [String: JSONValue]?
    ) -> VDMXObservableRestoreEntry {
        VDMXObservableRestoreEntry(
            path: path,
            classification: classification,
            reason: reason,
            capturedValue: capturedNode["VALUE"],
            liveValue: liveNode?["VALUE"]
        )
    }

    private static func flatten(_ root: [String: JSONValue]) -> [String: [String: JSONValue]] {
        var result: [String: [String: JSONValue]] = [:]

        func visit(_ node: [String: JSONValue]) {
            if let path = string(node["FULL_PATH"]), path.hasPrefix("/") {
                result[path] = node
            }
            guard case .object(let contents)? = node["CONTENTS"] else { return }
            for key in contents.keys.sorted() {
                if case .object(let child)? = contents[key] {
                    visit(child)
                }
            }
        }

        visit(root)
        return result
    }

    private static func isWritable(_ node: [String: JSONValue]) -> Bool {
        guard let access = integer(node["ACCESS"]) else { return false }
        return (access & 2) != 0
    }

    private static func defaultStatefulType(_ type: String) -> Bool {
        guard !type.isEmpty else { return false }
        return type.allSatisfy { $0 == "f" || $0 == "d" }
    }

    private static func compatibleValue(_ value: JSONValue?, withType type: String) -> Bool {
        guard case .array(let values)? = value,
              values.count == type.count
        else { return false }

        for (character, value) in zip(type, values) {
            switch character {
            case "f", "d":
                guard numeric(value) != nil else { return false }
            case "i", "h":
                guard integer(value) != nil else { return false }
            case "T", "F":
                guard case .bool = value else { return false }
            case "s":
                guard case .string = value else { return false }
            default:
                return false
            }
        }
        return true
    }

    private static func capturedValueWithinLiveRange(
        _ value: JSONValue?,
        liveRange: JSONValue?
    ) -> Bool {
        guard case .array(let values)? = value else { return false }
        // Automatic restore requires an explicit current live RANGE for every
        // numeric argument. Missing bounds are UNKNOWN, never permission to
        // write a captured value back into VDMX.
        guard let liveRange,
              case .array(let ranges) = liveRange
        else { return false }

        // OSCQuery permits RANGE metadata per argument. Missing entries do not
        // prove safety, so require each captured argument to have a usable
        // current range when RANGE is present.
        guard ranges.count >= values.count else { return false }

        for index in values.indices {
            guard let number = numeric(values[index]) else {
                // Non-numeric arguments have no numeric range semantics here.
                continue
            }
            guard case .object(let range) = ranges[index],
                  let minimum = numeric(range["MIN"]),
                  let maximum = numeric(range["MAX"]),
                  minimum <= maximum,
                  number >= minimum,
                  number <= maximum
            else {
                return false
            }
        }
        return true
    }

    private static func string(_ value: JSONValue?) -> String? {
        guard case .string(let text)? = value else { return nil }
        return text
    }

    private static func integer(_ value: JSONValue?) -> Int? {
        switch value {
        case .int(let number):
            return number
        case .double(let number)
            where number.isFinite
                && number.rounded(.towardZero) == number
                && number >= Double(Int.min)
                && number <= Double(Int.max):
            return Int(number)
        default:
            return nil
        }
    }

    private static func numeric(_ value: JSONValue?) -> Double? {
        switch value {
        case .int(let number):
            return Double(number)
        case .double(let number) where number.isFinite:
            return number
        default:
            return nil
        }
    }
}

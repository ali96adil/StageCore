#if os(macOS)
import CryptoKit
import Foundation

public struct VDMXInspectionProvider: CompanionInspectionProvider {
    public let adapterKey = "stagecore.adapter.vdmx"

    private let applicationCandidates: [URL]
    private let architecture: String

    public init(
        applicationCandidates: [URL] = Self.defaultApplicationCandidates(),
        architecture: String = Self.currentArchitecture
    ) {
        self.applicationCandidates = applicationCandidates
        self.architecture = architecture
    }

    public func inspect(manifest: [String: JSONValue]) async -> CompanionInspectionOutcome {
        do {
            try Task.checkCancellation()
            let decoded = try decodeManifest(manifest)
            guard decoded.adapterKey == adapterKey, decoded.application.key == "vdmx" else {
                return .init(
                    status: .failed,
                    errorCode: "VDMX_MANIFEST_INVALID",
                    responseSummary: "manifest is not a VDMX execution environment"
                )
            }
            if !decoded.externalExtensions.isEmpty || !decoded.bindings.isEmpty {
                return .init(
                    status: .failed,
                    errorCode: "VDMX_INSPECTION_SCOPE_UNSUPPORTED",
                    responseSummary: "VDMX extension and binding inspection is not implemented by this provider"
                )
            }

            let application = inspectApplication(constraint: decoded.application.versionConstraint)
            var assets: [CompanionInspectionAssetObservation] = []
            assets.reserveCapacity(decoded.assets.count)
            for asset in decoded.assets {
                try Task.checkCancellation()
                assets.append(try await inspectAsset(asset))
            }

            return .init(
                status: .completed,
                responseSummary: "VDMX application and declared asset inspection completed",
                observation: CompanionInspectionObservation(
                    os: "darwin",
                    architecture: architecture,
                    application: application,
                    assets: assets
                )
            )
        } catch is CancellationError {
            return .init(
                status: .failed,
                errorCode: "VDMX_INSPECTION_CANCELLED",
                responseSummary: "VDMX inspection was cancelled"
            )
        } catch {
            return .init(
                status: .failed,
                errorCode: "VDMX_INSPECTION_FAILED",
                responseSummary: "VDMX inspection could not decode or inspect the declared environment"
            )
        }
    }

    private func inspectApplication(constraint: String) -> CompanionInspectionApplicationObservation {
        for candidate in applicationCandidates {
            guard safeExistingURL(candidate) != nil else { continue }
            let infoURL = candidate
                .appendingPathComponent("Contents", isDirectory: true)
                .appendingPathComponent("Info.plist", isDirectory: false)
            guard safeExistingURL(infoURL) != nil,
                  let infoData = try? Data(contentsOf: infoURL),
                  let plist = try? PropertyListSerialization.propertyList(
                    from: infoData,
                    options: [],
                    format: nil
                  ) as? [String: Any]
            else {
                return .init(
                    present: true,
                    observedVersion: "",
                    versionConstraintSatisfied: nil
                )
            }

            let version = (plist["CFBundleShortVersionString"] as? String)
                ?? (plist["CFBundleVersion"] as? String)
                ?? ""
            let generation = applicationGeneration(from: candidate.lastPathComponent)
            return .init(
                present: true,
                observedVersion: version,
                versionConstraintSatisfied: versionCompatibility(
                    observedVersion: version,
                    generation: generation,
                    constraint: constraint
                )
            )
        }
        return .init(present: false)
    }

    private func inspectAsset(_ asset: VDMXAssetRequirement) async throws -> CompanionInspectionAssetObservation {
        let fileManager = FileManager.default
        guard let url = declaredFileURL(asset.locator) else {
            return .init(key: asset.key, present: false, inspectable: false)
        }
        var isDirectory: ObjCBool = false
        guard fileManager.fileExists(atPath: url.path, isDirectory: &isDirectory) else {
            return .init(key: asset.key, present: false, inspectable: false)
        }
        guard safeExistingURL(url) != nil else {
            return .init(key: asset.key, present: false, inspectable: false)
        }

        switch asset.capturePolicy {
        case "REFERENCE_ONLY":
            return .init(key: asset.key, present: true, inspectable: false)
        case "CONTENT_BOUND":
            guard !isDirectory.boolValue else {
                return .init(key: asset.key, present: true, inspectable: false)
            }
            guard let attributes = try? fileManager.attributesOfItem(atPath: url.path),
                  let size = (attributes[.size] as? NSNumber)?.int64Value
            else {
                return .init(key: asset.key, present: true, inspectable: false)
            }
            do {
                let hash = try await sha256Hex(of: url)
                return .init(
                    key: asset.key,
                    present: true,
                    inspectable: true,
                    contentHash: hash,
                    sizeBytes: size
                )
            } catch is CancellationError {
                throw CancellationError()
            } catch {
                return .init(key: asset.key, present: true, inspectable: false)
            }
        default:
            return .init(key: asset.key, present: false, inspectable: false)
        }
    }

    private func sha256Hex(of url: URL) async throws -> String {
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        var hasher = SHA256()
        while true {
            try Task.checkCancellation()
            let data = try handle.read(upToCount: 1024 * 1024) ?? Data()
            if data.isEmpty { break }
            hasher.update(data: data)
        }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    private func safeExistingURL(_ url: URL) -> URL? {
        let standardized = url.standardizedFileURL
        guard FileManager.default.fileExists(atPath: standardized.path) else { return nil }
        let resolved = standardized.resolvingSymlinksInPath().standardizedFileURL
        guard standardized.path == resolved.path else { return nil }
        return standardized
    }

    private func declaredFileURL(_ locator: String) -> URL? {
        if locator.hasPrefix("file://") {
            guard let url = URL(string: locator), url.isFileURL else { return nil }
            return url.standardizedFileURL
        }
        guard locator.hasPrefix("/") else { return nil }
        return URL(fileURLWithPath: locator, isDirectory: false).standardizedFileURL
    }

    private func decodeManifest(_ manifest: [String: JSONValue]) throws -> VDMXManifest {
        let data = try JSONEncoder().encode(manifest)
        return try JSONDecoder().decode(VDMXManifest.self, from: data)
    }

    private func applicationGeneration(from name: String) -> Int? {
        let lower = name.lowercased()
        guard let range = lower.range(of: "vdmx") else { return nil }
        let suffix = lower[range.upperBound...]
        let digits = suffix.prefix { $0.isNumber }
        return Int(digits)
    }

    private func versionCompatibility(
        observedVersion: String,
        generation: Int?,
        constraint rawConstraint: String
    ) -> Bool? {
        let constraint = rawConstraint.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if constraint.hasSuffix(".x-tested") {
            let majorText = String(constraint.dropLast(".x-tested".count))
            guard let expectedGeneration = Int(majorText), let generation else { return nil }
            return generation == expectedGeneration
        }
        if constraint.hasPrefix(">=") {
            let required = String(constraint.dropFirst(2))
            guard let observed = numericVersion(observedVersion),
                  let minimum = numericVersion(required)
            else { return nil }
            return compareVersions(observed, minimum) >= 0
        }
        guard let observed = numericVersion(observedVersion),
              let required = numericVersion(constraint)
        else { return nil }
        return compareVersions(observed, required) == 0
    }

    private func numericVersion(_ value: String) -> [Int]? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        let components = trimmed.split(separator: ".", omittingEmptySubsequences: false)
        guard !components.isEmpty else { return nil }
        var result: [Int] = []
        result.reserveCapacity(components.count)
        for component in components {
            guard !component.isEmpty,
                  component.allSatisfy({ $0.isNumber }),
                  let number = Int(component)
            else { return nil }
            result.append(number)
        }
        return result
    }

    private func compareVersions(_ lhs: [Int], _ rhs: [Int]) -> Int {
        let count = max(lhs.count, rhs.count)
        for index in 0..<count {
            let left = index < lhs.count ? lhs[index] : 0
            let right = index < rhs.count ? rhs[index] : 0
            if left < right { return -1 }
            if left > right { return 1 }
        }
        return 0
    }

    public static func defaultApplicationCandidates(
        homeDirectory: URL = FileManager.default.homeDirectoryForCurrentUser
    ) -> [URL] {
        let system = URL(fileURLWithPath: "/Applications", isDirectory: true)
        let user = homeDirectory.appendingPathComponent("Applications", isDirectory: true)
        return [
            system.appendingPathComponent("VDMX6 Plus.app", isDirectory: true),
            system.appendingPathComponent("VDMX6.app", isDirectory: true),
            user.appendingPathComponent("VDMX6 Plus.app", isDirectory: true),
            user.appendingPathComponent("VDMX6.app", isDirectory: true),
        ]
    }

    public static var currentArchitecture: String {
        #if arch(arm64)
        return "arm64"
        #elseif arch(x86_64)
        return "amd64"
        #else
        return "unknown"
        #endif
    }
}

private struct VDMXManifest: Decodable {
    let adapterKey: String
    let application: VDMXApplicationRequirement
    let assets: [VDMXAssetRequirement]
    let externalExtensions: [VDMXExtensionRequirement]
    let bindings: [VDMXBindingRequirement]

    enum CodingKeys: String, CodingKey {
        case adapterKey = "adapter_key"
        case application
        case assets
        case externalExtensions = "external_extensions"
        case bindings
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        adapterKey = try container.decode(String.self, forKey: .adapterKey)
        application = try container.decode(VDMXApplicationRequirement.self, forKey: .application)
        assets = try container.decodeIfPresent([VDMXAssetRequirement].self, forKey: .assets) ?? []
        externalExtensions = try container.decodeIfPresent([VDMXExtensionRequirement].self, forKey: .externalExtensions) ?? []
        bindings = try container.decodeIfPresent([VDMXBindingRequirement].self, forKey: .bindings) ?? []
    }
}

private struct VDMXApplicationRequirement: Decodable {
    let key: String
    let versionConstraint: String

    enum CodingKeys: String, CodingKey {
        case key
        case versionConstraint = "version_constraint"
    }
}

private struct VDMXAssetRequirement: Decodable {
    let key: String
    let capturePolicy: String
    let locator: String

    enum CodingKeys: String, CodingKey {
        case key
        case capturePolicy = "capture_policy"
        case locator
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        key = try container.decode(String.self, forKey: .key)
        capturePolicy = try container.decode(String.self, forKey: .capturePolicy)
        locator = try container.decodeIfPresent(String.self, forKey: .locator) ?? ""
    }
}

private struct VDMXExtensionRequirement: Decodable {}
private struct VDMXBindingRequirement: Decodable {}
#endif

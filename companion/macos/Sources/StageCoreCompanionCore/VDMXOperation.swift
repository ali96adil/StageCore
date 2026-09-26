import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
#if os(macOS)
import AppKit
#endif

public typealias VDMXOpenHandler = @Sendable (_ target: URL, _ application: URL) async -> Bool
typealias VDMXApplicationOpenHandler = @Sendable (_ application: URL) async -> Bool
typealias VDMXOSCQueryFetcher = @Sendable (_ url: URL) async throws -> Data

public struct VDMXOperationProvider: ExecutionEnvironmentOperationProvider {
    public let adapterKey = "stagecore.adapter.vdmx"
    public let supportedOperations: Set<ExecutionEnvironmentOperationKind> = [.open, .captureSnapshot]

    private let applicationCandidates: [URL]
    private let opener: VDMXOpenHandler
    private let applicationOpener: VDMXApplicationOpenHandler
    private let oscQueryFetcher: VDMXOSCQueryFetcher

    private static let maxOSCQueryNamespaceBytes = 40 * 1024
    private static let maxOSCQueryHostInfoBytes = 4 * 1024

    public init(
        applicationCandidates: [URL],
        opener: @escaping VDMXOpenHandler
    ) {
        self.applicationCandidates = applicationCandidates
        self.opener = opener
        self.applicationOpener = { _ in false }
        self.oscQueryFetcher = { url in
            try await Self.fetchOSCQuery(url)
        }
    }

    init(
        applicationCandidates: [URL],
        opener: @escaping VDMXOpenHandler,
        applicationOpener: @escaping VDMXApplicationOpenHandler = { _ in false },
        oscQueryFetcher: @escaping VDMXOSCQueryFetcher
    ) {
        self.applicationCandidates = applicationCandidates
        self.opener = opener
        self.applicationOpener = applicationOpener
        self.oscQueryFetcher = oscQueryFetcher
    }

    #if os(macOS)
    public init(applicationCandidates: [URL] = VDMXInspectionProvider.defaultApplicationCandidates()) {
        self.init(
            applicationCandidates: applicationCandidates,
            opener: { target, application in
                await Self.openWithNSWorkspace(target: target, application: application)
            }
        )
    }
    #endif

    public func perform(
        kind: ExecutionEnvironmentOperationKind,
        manifest: [String: JSONValue],
        sourceManifestSHA256: String
    ) async -> ExecutionEnvironmentProviderOutcome {
        do {
            try Task.checkCancellation()
            let decoded = try decodeManifest(manifest)
            guard decoded.schemaVersion == 1,
                  decoded.adapterKey == adapterKey,
                  decoded.application.key == "vdmx",
                  isSHA256(sourceManifestSHA256)
            else {
                return failure(
                    code: "VDMX_MANIFEST_INVALID",
                    summary: "manifest is not a valid VDMX execution environment"
                )
            }

            switch kind {
            case .open:
                return await performOpen(manifest: decoded)
            case .captureSnapshot:
                return await captureSnapshot(manifest: decoded, sourceManifestSHA256: sourceManifestSHA256.lowercased())
            case .reconnect:
                return .init(
                    status: .unsupported,
                    errorCode: "ENVIRONMENT_OPERATION_UNSUPPORTED",
                    responseSummary: "VDMX reconnect is not exposed through a supported operation surface"
                )
            }
        } catch is CancellationError {
            return failure(code: "VDMX_OPERATION_CANCELLED", summary: "VDMX operation was cancelled")
        } catch {
            return failure(code: "VDMX_MANIFEST_INVALID", summary: "VDMX operation could not decode the execution environment manifest")
        }
    }

    private func performOpen(manifest: VDMXOperationManifest) async -> ExecutionEnvironmentProviderOutcome {
        guard let application = locateApplication() else {
            return failure(code: "VDMX_APPLICATION_NOT_FOUND", summary: "VDMX application bundle was not found at a safe known location")
        }
        if manifest.launch == nil {
            if Task.isCancelled {
                return failure(code: "VDMX_OPERATION_CANCELLED", summary: "VDMX operation was cancelled")
            }
            guard await applicationOpener(application) else {
                return failure(code: "VDMX_OPEN_FAILED", summary: "macOS could not open VDMX")
            }
            return .init(
                status: .completed,
                responseSummary: "VDMX opened without a saved workspace target"
            )
        }
        guard let target = resolveLaunchTarget(manifest),
              safeExistingURL(target) != nil
        else {
            return failure(code: "VDMX_LAUNCH_TARGET_UNAVAILABLE", summary: "declared VDMX launch target is missing or cannot be opened safely")
        }
        if Task.isCancelled {
            return failure(code: "VDMX_OPERATION_CANCELLED", summary: "VDMX operation was cancelled")
        }
        guard await opener(target, application) else {
            return failure(code: "VDMX_OPEN_FAILED", summary: "macOS could not open the declared target with VDMX")
        }
        return .init(
            status: .completed,
            responseSummary: "VDMX opened the declared execution-environment launch target"
        )
    }

    private func captureSnapshot(
        manifest: VDMXOperationManifest,
        sourceManifestSHA256: String
    ) async -> ExecutionEnvironmentProviderOutcome {
        let application = locateApplication()
        var items: [[String: JSONValue]] = []

        items.append([
            "key": .string("vdmx-application"),
            "name": .string("VDMX application"),
            "kind": .string("OTHER"),
            "provenance": .string("ADAPTER_OBSERVATION"),
            "capture_status": .string(application == nil ? "MISSING" : "OBSERVED"),
            "portability": .string("DESCRIPTIVE_ONLY"),
            "notes": .string(application == nil
                ? "VDMX application bundle was not found at a safe known location."
                : "VDMX application bundle was found; application installation remains destination-specific."),
        ])

        if let launch = launchLocator(manifest) {
            if let target = declaredFileURL(launch), safeExistingURL(target) != nil {
                items.append([
                    "key": .string("declared-launch-target"),
                    "name": .string("Declared VDMX launch target"),
                    "kind": .string("REFERENCE_MATERIAL"),
                    "provenance": .string("ADAPTER_OBSERVATION"),
                    "capture_status": .string("OBSERVED"),
                    "portability": .string("REFERENCE_ONLY"),
                    "locator": .string(launch),
                    "notes": .string("Target was observed in place. This snapshot does not claim possession of its bytes."),
                ])
            } else if declaredFileURL(launch) != nil {
                items.append([
                    "key": .string("declared-launch-target"),
                    "name": .string("Declared VDMX launch target"),
                    "kind": .string("REFERENCE_MATERIAL"),
                    "provenance": .string("ADAPTER_OBSERVATION"),
                    "capture_status": .string("MISSING"),
                    "portability": .string("REFERENCE_ONLY"),
                    "locator": .string(launch),
                    "notes": .string("Declared target was not safely present at capture time."),
                ])
            } else {
                items.append(unsupportedLaunchItem())
            }
        } else {
            items.append(unsupportedLaunchItem())
        }

        if let oscQuery = await captureOSCQuery(manifest: manifest) {
            items.append(oscQuery)
        }

        items.append([
            "key": .string("vdmx-internal-state"),
            "name": .string("VDMX internal workspace and published-control state"),
            "kind": .string("CONTROL_STATE"),
            "provenance": .string("ADAPTER_OBSERVATION"),
            "capture_status": .string("UNSUPPORTED"),
            "portability": .string("DESCRIPTIVE_ONLY"),
            "notes": .string("This provider does not claim complete VDMX internal workspace, plugin, FX, or published-control state capture."),
        ])

        let snapshot: [String: JSONValue] = [
            "schema_version": .int(1),
            "environment_key": .string(manifest.environmentKey),
            "adapter_key": .string(adapterKey),
            "source_manifest_sha256": .string(sourceManifestSHA256),
            "capture_status": .string("PARTIAL"),
            "items": .array(items.map(JSONValue.object)),
            "notes": .string("Truthful partial VDMX reconstruction snapshot. Managed content bytes remain authoritative in the StageCore Vault; destination readiness requires fresh inspection."),
        ]
        return .init(
            status: .completed,
            responseSummary: "VDMX partial execution-environment snapshot captured",
            snapshot: snapshot
        )
    }

    private func captureOSCQuery(manifest: VDMXOperationManifest) async -> [String: JSONValue]? {
        guard let binding = manifest.bindings.first(where: {
            ($0.key == "oscquery" || $0.key == "vdmx-oscquery") && $0.kind == "NETWORK"
        }) else {
            return nil
        }

        guard let endpoint = validatedLocalOSCQueryEndpoint(binding.externalRef) else {
            return oscQueryItem(
                capture: "UNSUPPORTED",
                notes: "Declared OSCQuery endpoint is not a supported loopback HTTP URL."
            )
        }

        do {
            let namespaceData = try await oscQueryFetcher(endpoint)
            guard namespaceData.count <= Self.maxOSCQueryNamespaceBytes,
                  let namespace = try decodeJSONObject(namespaceData)
            else {
                return oscQueryItem(
                    capture: "UNSUPPORTED",
                    notes: "OSCQuery namespace exceeded the bounded capture size or was not a JSON object."
                )
            }

            var metadata: [String: JSONValue] = [
                "endpoint": .string(endpoint.absoluteString),
                "namespace": .object(namespace),
                "published_node_count": .int(countOSCQueryNodes(namespace)),
            ]

            if let hostInfoURL = hostInfoURL(endpoint) {
                do {
                    let hostInfoData = try await oscQueryFetcher(hostInfoURL)
                    if hostInfoData.count <= Self.maxOSCQueryHostInfoBytes,
                       let hostInfo = try decodeJSONObject(hostInfoData) {
                        metadata["host_info"] = .object(hostInfo)
                    }
                } catch {
                    // Namespace capture remains useful and truthful without optional HOST_INFO.
                }
            }

            return [
                "key": .string("vdmx-oscquery"),
                "name": .string("VDMX published OSCQuery namespace"),
                "kind": .string("CONTROL_NAMESPACE"),
                "provenance": .string("OSCQUERY"),
                "capture_status": .string("OBSERVED"),
                "portability": .string("DESCRIPTIVE_ONLY"),
                "notes": .string("Read-only localhost OSCQuery namespace published by VDMX. This covers only controls VDMX exposes through OSCQuery."),
                "metadata": .object(metadata),
            ]
        } catch {
            return oscQueryItem(
                capture: "MISSING",
                notes: "Declared local VDMX OSCQuery endpoint was unavailable at capture time."
            )
        }
    }

    private func oscQueryItem(capture: String, notes: String) -> [String: JSONValue] {
        [
            "key": .string("vdmx-oscquery"),
            "name": .string("VDMX published OSCQuery namespace"),
            "kind": .string("CONTROL_NAMESPACE"),
            "provenance": .string("OSCQUERY"),
            "capture_status": .string(capture),
            "portability": .string("DESCRIPTIVE_ONLY"),
            "notes": .string(notes),
        ]
    }

    private func validatedLocalOSCQueryEndpoint(_ raw: String) -> URL? {
        guard let url = URL(string: raw),
              url.scheme?.lowercased() == "http",
              let host = url.host?.lowercased(),
              host == "127.0.0.1" || host == "localhost" || host == "::1",
              url.user == nil,
              url.password == nil,
              url.fragment == nil,
              url.query == nil
        else {
            return nil
        }
        return url
    }

    private func hostInfoURL(_ endpoint: URL) -> URL? {
        guard var components = URLComponents(url: endpoint, resolvingAgainstBaseURL: false) else {
            return nil
        }
        components.query = "HOST_INFO"
        return components.url
    }

    private func decodeJSONObject(_ data: Data) throws -> [String: JSONValue]? {
        try JSONDecoder().decode([String: JSONValue].self, from: data)
    }

    private func countOSCQueryNodes(_ object: [String: JSONValue]) -> Int {
        var total = object["FULL_PATH"] == nil ? 0 : 1
        if case .object(let contents)? = object["CONTENTS"] {
            for value in contents.values {
                if case .object(let child) = value {
                    total += countOSCQueryNodes(child)
                }
            }
        }
        return total
    }

    private static func fetchOSCQuery(_ url: URL) async throws -> Data {
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 1.5
        request.cachePolicy = .reloadIgnoringLocalCacheData
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            throw URLError(.badServerResponse)
        }
        return data
    }

    private func unsupportedLaunchItem() -> [String: JSONValue] {
        [
            "key": .string("declared-launch-target"),
            "name": .string("Declared VDMX launch target"),
            "kind": .string("REFERENCE_MATERIAL"),
            "provenance": .string("ADAPTER_OBSERVATION"),
            "capture_status": .string("UNSUPPORTED"),
            "portability": .string("DESCRIPTIVE_ONLY"),
            "notes": .string("The declared launch target is not a safely inspectable local absolute path or file URL."),
        ]
    }

    private func locateApplication() -> URL? {
        for candidate in applicationCandidates {
            if let safe = safeExistingURL(candidate) { return safe }
        }
        return nil
    }

    private func resolveLaunchTarget(_ manifest: VDMXOperationManifest) -> URL? {
        guard let locator = launchLocator(manifest) else { return nil }
        return declaredFileURL(locator)
    }

    private func launchLocator(_ manifest: VDMXOperationManifest) -> String? {
        guard let launch = manifest.launch else { return nil }
        switch launch.kind {
        case "ASSET":
            guard let assetKey = launch.assetKey,
                  let asset = manifest.assets.first(where: { $0.key == assetKey }),
                  !asset.locator.isEmpty
            else { return nil }
            return asset.locator
        case "LOCATOR":
            guard let locator = launch.locator, !locator.isEmpty else { return nil }
            return locator
        default:
            return nil
        }
    }

    private func declaredFileURL(_ locator: String) -> URL? {
        if locator.hasPrefix("file://") {
            guard let url = URL(string: locator), url.isFileURL else { return nil }
            return url.standardizedFileURL
        }
        guard locator.hasPrefix("/") else { return nil }
        return URL(fileURLWithPath: locator, isDirectory: false).standardizedFileURL
    }

    private func safeExistingURL(_ url: URL) -> URL? {
        let standardized = url.standardizedFileURL
        guard FileManager.default.fileExists(atPath: standardized.path) else { return nil }
        let resolved = standardized.resolvingSymlinksInPath().standardizedFileURL
        guard standardized.path == resolved.path else { return nil }
        return standardized
    }

    private func isSHA256(_ value: String) -> Bool {
        value.count == 64 && value.allSatisfy { $0.isHexDigit }
    }

    private func decodeManifest(_ manifest: [String: JSONValue]) throws -> VDMXOperationManifest {
        let data = try JSONEncoder().encode(manifest)
        return try JSONDecoder().decode(VDMXOperationManifest.self, from: data)
    }

    private func failure(code: String, summary: String) -> ExecutionEnvironmentProviderOutcome {
        .init(status: .failed, errorCode: code, responseSummary: summary)
    }

    #if os(macOS)
    @MainActor
    private static func openApplicationWithNSWorkspace(application: URL) async -> Bool {
        await withCheckedContinuation { continuation in
            let configuration = NSWorkspace.OpenConfiguration()
            NSWorkspace.shared.openApplication(
                at: application,
                configuration: configuration
            ) { runningApplication, error in
                continuation.resume(returning: runningApplication != nil && error == nil)
            }
        }
    }

    @MainActor
    private static func openWithNSWorkspace(target: URL, application: URL) async -> Bool {
        await withCheckedContinuation { continuation in
            let configuration = NSWorkspace.OpenConfiguration()
            NSWorkspace.shared.open(
                [target],
                withApplicationAt: application,
                configuration: configuration
            ) { runningApplication, error in
                continuation.resume(returning: runningApplication != nil && error == nil)
            }
        }
    }
    #endif
}

private struct VDMXOperationManifest: Decodable {
    let schemaVersion: Int
    let environmentKey: String
    let adapterKey: String
    let application: VDMXOperationApplication
    let assets: [VDMXOperationAsset]
    let bindings: [VDMXOperationBinding]
    let launch: VDMXOperationLaunch?

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case environmentKey = "environment_key"
        case adapterKey = "adapter_key"
        case application
        case assets
        case bindings
        case launch
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        schemaVersion = try container.decode(Int.self, forKey: .schemaVersion)
        environmentKey = try container.decode(String.self, forKey: .environmentKey)
        adapterKey = try container.decode(String.self, forKey: .adapterKey)
        application = try container.decode(VDMXOperationApplication.self, forKey: .application)
        assets = try container.decodeIfPresent([VDMXOperationAsset].self, forKey: .assets) ?? []
        bindings = try container.decodeIfPresent([VDMXOperationBinding].self, forKey: .bindings) ?? []
        launch = try container.decodeIfPresent(VDMXOperationLaunch.self, forKey: .launch)
    }
}

private struct VDMXOperationApplication: Decodable {
    let key: String
}

private struct VDMXOperationAsset: Decodable {
    let key: String
    let locator: String

    enum CodingKeys: String, CodingKey {
        case key
        case locator
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        key = try container.decode(String.self, forKey: .key)
        locator = try container.decodeIfPresent(String.self, forKey: .locator) ?? ""
    }
}

private struct VDMXOperationBinding: Decodable {
    let key: String
    let kind: String
    let externalRef: String

    enum CodingKeys: String, CodingKey {
        case key
        case kind
        case externalRef = "external_ref"
    }
}

private struct VDMXOperationLaunch: Decodable {
    let kind: String
    let assetKey: String?
    let locator: String?

    enum CodingKeys: String, CodingKey {
        case kind
        case assetKey = "asset_key"
        case locator
    }
}

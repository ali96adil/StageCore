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
typealias VDMXOSCDatagramSender = @Sendable (_ packet: Data, _ endpoint: OSCEndpoint) async throws -> Int

public struct VDMXOperationProvider: ExecutionEnvironmentOperationProvider {
    public let adapterKey = "stagecore.adapter.vdmx"
    public let supportedOperations: Set<ExecutionEnvironmentOperationKind> = [.open, .captureSnapshot, .restoreObservableState]

    private let applicationCandidates: [URL]
    private let opener: VDMXOpenHandler
    private let applicationOpener: VDMXApplicationOpenHandler
    private let oscQueryFetcher: VDMXOSCQueryFetcher
    private let oscSender: VDMXOSCDatagramSender

    private static let maxOSCQueryNamespaceBytes = 40 * 1024
    private static let maxOSCQueryHostInfoBytes = 4 * 1024
    private static let maxObservableRestoreWrites = 128

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
        self.oscSender = { packet, endpoint in
            try POSIXOSCDatagramSender().send(packet, to: endpoint)
        }
    }

    init(
        applicationCandidates: [URL],
        opener: @escaping VDMXOpenHandler,
        applicationOpener: @escaping VDMXApplicationOpenHandler = { _ in false },
        oscQueryFetcher: @escaping VDMXOSCQueryFetcher,
        oscSender: @escaping VDMXOSCDatagramSender = { packet, endpoint in
            try POSIXOSCDatagramSender().send(packet, to: endpoint)
        }
    ) {
        self.applicationCandidates = applicationCandidates
        self.opener = opener
        self.applicationOpener = applicationOpener
        self.oscQueryFetcher = oscQueryFetcher
        self.oscSender = oscSender
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
        sourceManifestSHA256: String,
        snapshot: [String: JSONValue]?
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
            case .restoreObservableState:
                guard let snapshot else {
                    return failure(
                        code: "VDMX_RESTORE_SNAPSHOT_REQUIRED",
                        summary: "VDMX observable-state restore requires a Hub-selected snapshot"
                    )
                }
                return await restoreObservableState(
                    manifest: decoded,
                    sourceManifestSHA256: sourceManifestSHA256.lowercased(),
                    snapshot: snapshot
                )
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

    private func restoreObservableState(
        manifest: VDMXOperationManifest,
        sourceManifestSHA256: String,
        snapshot: [String: JSONValue]
    ) async -> ExecutionEnvironmentProviderOutcome {
        guard let capturedNamespace = capturedOSCQueryNamespace(
            snapshot: snapshot,
            manifest: manifest,
            sourceManifestSHA256: sourceManifestSHA256
        ) else {
            return failure(
                code: "VDMX_RESTORE_SNAPSHOT_INVALID",
                summary: "snapshot does not contain a matching observed VDMX OSCQuery namespace"
            )
        }

        do {
            let first = try await readLiveOSCQuery(manifest: manifest)
            let firstPreview = VDMXObservableRestorePlanner.preview(
                capturedNamespace: capturedNamespace,
                liveNamespace: first.namespace
            )
            if let reason = restorePreviewBlocker(firstPreview) {
                return failure(
                    code: "VDMX_RESTORE_LIVE_SURFACE_UNSAFE",
                    summary: reason
                )
            }

            let candidates = firstPreview.entries.filter {
                $0.classification == .restorable
            }
            guard candidates.count <= Self.maxObservableRestoreWrites else {
                return failure(
                    code: "VDMX_RESTORE_TOO_MANY_WRITES",
                    summary: "observable-state restore exceeds the bounded write count"
                )
            }
            if candidates.isEmpty {
                return .init(
                    status: .completed,
                    responseSummary: "VDMX observable state already matches or contains only controls excluded from automatic restore"
                )
            }

            // Race fence: read the live namespace again immediately before the
            // first datagram. Any classification/value change aborts before
            // mutating VDMX.
            let second = try await readLiveOSCQuery(manifest: manifest)
            guard first.oscEndpoint == second.oscEndpoint else {
                return failure(
                    code: "VDMX_RESTORE_LIVE_SURFACE_CHANGED",
                    summary: "VDMX OSC endpoint changed during restore preflight"
                )
            }
            let secondPreview = VDMXObservableRestorePlanner.preview(
                capturedNamespace: capturedNamespace,
                liveNamespace: second.namespace
            )
            guard firstPreview.entries == secondPreview.entries else {
                return failure(
                    code: "VDMX_RESTORE_LIVE_SURFACE_CHANGED",
                    summary: "VDMX published state changed during restore preflight"
                )
            }

            let capturedNodes = flattenOSCQueryNodes(capturedNamespace)
            for candidate in candidates.sorted(by: { $0.path < $1.path }) {
                try Task.checkCancellation()
                guard let node = capturedNodes[candidate.path] else {
                    return failure(
                        code: "VDMX_RESTORE_SNAPSHOT_INVALID",
                        summary: "captured restore path disappeared from snapshot state"
                    )
                }
                let packet = try encodeRestorePacket(path: candidate.path, node: node)
                let sent = try await oscSender(packet, first.oscEndpoint)
                guard sent == packet.count else {
                    return failure(
                        code: "VDMX_RESTORE_SEND_FAILED",
                        summary: "VDMX OSC restore datagram was not fully sent; re-inspect state before retry"
                    )
                }
            }

            // Transport send is not proof of application state. Re-read VDMX
            // and require every path we actually wrote to report the captured
            // value before calling the operation completed.
            let verification = try await readLiveOSCQuery(manifest: manifest)
            let verificationPreview = VDMXObservableRestorePlanner.preview(
                capturedNamespace: capturedNamespace,
                liveNamespace: verification.namespace
            )
            let verificationByPath = Dictionary(
                uniqueKeysWithValues: verificationPreview.entries.map { ($0.path, $0) }
            )
            for candidate in candidates {
                guard verificationByPath[candidate.path]?.classification == .alreadyMatching else {
                    return failure(
                        code: "VDMX_RESTORE_VERIFICATION_FAILED",
                        summary: "VDMX did not report the requested observable state after restore; re-inspect before retry"
                    )
                }
            }

            return .init(
                status: .completed,
                responseSummary: "VDMX restored and re-verified \(candidates.count) stateful published control(s); discrete/event controls were not replayed"
            )
        } catch is CancellationError {
            return failure(
                code: "VDMX_OPERATION_CANCELLED",
                summary: "VDMX observable-state restore was cancelled; re-inspect state before retry"
            )
        } catch {
            return failure(
                code: "VDMX_RESTORE_FAILED",
                summary: "VDMX observable-state restore failed; re-inspect state before retry"
            )
        }
    }

    private func capturedOSCQueryNamespace(
        snapshot: [String: JSONValue],
        manifest: VDMXOperationManifest,
        sourceManifestSHA256: String
    ) -> [String: JSONValue]? {
        guard snapshot["schema_version"] == .int(1),
              snapshot["environment_key"] == .string(manifest.environmentKey),
              snapshot["adapter_key"] == .string(adapterKey),
              case .string(let sourceHash)? = snapshot["source_manifest_sha256"],
              sourceHash.lowercased() == sourceManifestSHA256,
              case .array(let items)? = snapshot["items"]
        else {
            return nil
        }

        var matches: [[String: JSONValue]] = []
        for item in items {
            guard case .object(let object) = item,
                  object["key"] == .string("vdmx-oscquery")
            else { continue }
            matches.append(object)
        }
        guard matches.count == 1 else { return nil }
        let item = matches[0]
        guard item["kind"] == .string("CONTROL_NAMESPACE"),
              item["provenance"] == .string("OSCQUERY"),
              item["provenance_class"] == .string("OBSERVED"),
              item["capture_status"] == .string("OBSERVED"),
              item["portability"] == .string("DESCRIPTIVE_ONLY"),
              case .object(let metadata)? = item["metadata"],
              case .object(let namespace)? = metadata["namespace"]
        else {
            return nil
        }
        return namespace
    }

    private struct LiveOSCQueryState {
        let namespace: [String: JSONValue]
        let oscEndpoint: OSCEndpoint
    }

    private func readLiveOSCQuery(
        manifest: VDMXOperationManifest
    ) async throws -> LiveOSCQueryState {
        guard let binding = manifest.bindings.first(where: {
            ($0.key == "oscquery" || $0.key == "vdmx-oscquery") && $0.kind == "NETWORK"
        }),
        let endpoint = validatedLocalOSCQueryEndpoint(binding.externalRef)
        else {
            throw VDMXRestoreError.invalidEndpoint
        }

        let namespaceData = try await oscQueryFetcher(endpoint)
        guard namespaceData.count <= Self.maxOSCQueryNamespaceBytes,
              let namespace = try decodeJSONObject(namespaceData)
        else {
            throw VDMXRestoreError.invalidNamespace
        }

        guard let hostURL = hostInfoURL(endpoint) else {
            throw VDMXRestoreError.invalidHostInfo
        }
        let hostData = try await oscQueryFetcher(hostURL)
        guard hostData.count <= Self.maxOSCQueryHostInfoBytes,
              let hostInfo = try decodeJSONObject(hostData),
              let port = oscPort(hostInfo),
              (1...65535).contains(port)
        else {
            throw VDMXRestoreError.invalidHostInfo
        }

        return LiveOSCQueryState(
            namespace: namespace,
            oscEndpoint: OSCEndpoint(host: "127.0.0.1", port: port)
        )
    }

    private func restorePreviewBlocker(
        _ preview: VDMXObservableRestorePreview
    ) -> String? {
        if preview.missingCount > 0 {
            return "captured VDMX OSC paths are missing from the current published namespace"
        }
        if preview.incompatibleCount > 0 {
            return "captured VDMX OSC types, values, or ranges are incompatible with the current published namespace"
        }
        if preview.readOnlyCount > 0 {
            return "one or more captured VDMX stateful controls are currently read-only"
        }
        return nil
    }

    private func flattenOSCQueryNodes(
        _ root: [String: JSONValue]
    ) -> [String: [String: JSONValue]] {
        var nodes: [String: [String: JSONValue]] = [:]
        func visit(_ node: [String: JSONValue]) {
            if case .string(let path)? = node["FULL_PATH"], path.hasPrefix("/") {
                nodes[path] = node
            }
            guard case .object(let contents)? = node["CONTENTS"] else { return }
            for key in contents.keys.sorted() {
                if case .object(let child)? = contents[key] {
                    visit(child)
                }
            }
        }
        visit(root)
        return nodes
    }

    private func encodeRestorePacket(
        path: String,
        node: [String: JSONValue]
    ) throws -> Data {
        guard case .string(let type)? = node["TYPE"],
              case .array(let values)? = node["VALUE"],
              type.count == values.count,
              type.allSatisfy({ $0 == "f" || $0 == "d" })
        else {
            throw VDMXRestoreError.unsupportedType
        }

        var arguments: [JSONValue] = []
        for (tag, value) in zip(type, values) {
            switch (tag, value) {
            case ("f", .double(let number)):
                arguments.append(.object(["type": .string("float32"), "value": .double(number)]))
            case ("f", .int(let number)):
                arguments.append(.object(["type": .string("float32"), "value": .int(number)]))
            case ("d", .double(let number)):
                arguments.append(.object(["type": .string("float64"), "value": .double(number)]))
            case ("d", .int(let number)):
                arguments.append(.object(["type": .string("float64"), "value": .int(number)]))
            default:
                throw VDMXRestoreError.unsupportedType
            }
        }

        return try OSCPacketEncoder.encode(parameters: [
            "address": .string(path),
            "arguments": .array(arguments),
        ])
    }

    private func oscPort(_ hostInfo: [String: JSONValue]) -> Int? {
        switch hostInfo["OSC_PORT"] {
        case .some(.int(let port)):
            return port
        case .some(.double(let port))
            where port.isFinite
                && port.rounded(.towardZero) == port
                && port >= 1
                && port <= 65535:
            return Int(port)
        default:
            return nil
        }
    }

    private func captureSnapshot(
        manifest: VDMXOperationManifest,
        sourceManifestSHA256: String
    ) async -> ExecutionEnvironmentProviderOutcome {
        let application = locateApplication()
        var items: [[String: JSONValue]] = []

        items.append(applicationCaptureItem(application))

        if let launch = launchLocator(manifest) {
            if let target = declaredFileURL(launch), safeExistingURL(target) != nil {
                items.append([
                    "key": .string("declared-launch-target"),
                    "name": .string("Declared VDMX launch target"),
                    "kind": .string("REFERENCE_MATERIAL"),
                    "provenance": .string("ADAPTER_OBSERVATION"),
                    "provenance_class": .string("REFERENCE_ONLY"),
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
                    "provenance_class": .string("REFERENCE_ONLY"),
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
            "provenance_class": .string("UNSUPPORTED"),
            "capture_status": .string("UNSUPPORTED"),
            "portability": .string("DESCRIPTIVE_ONLY"),
            "notes": .string("This provider does not claim complete VDMX internal workspace, plugin, FX, or published-control state capture."),
        ])

        let itemValues = items.map(JSONValue.object)
        let plan = rebuildPlan(
            applicationPresent: application != nil,
            savedLaunchObserved: itemHasStatus(items, key: "declared-launch-target", status: "OBSERVED"),
            oscQueryObserved: itemHasStatus(items, key: "vdmx-oscquery", status: "OBSERVED")
        )
        let fingerprint = reconstructionFingerprint(items: itemValues, rebuildPlan: plan)

        let snapshot: [String: JSONValue] = [
            "schema_version": .int(1),
            "environment_key": .string(manifest.environmentKey),
            "adapter_key": .string(adapterKey),
            "source_manifest_sha256": .string(sourceManifestSHA256),
            "capture_status": .string("PARTIAL"),
            "rebuild_plan_version": .int(1),
            "reconstruction_fingerprint": .string(fingerprint),
            "rebuild_plan": .array(plan),
            "items": .array(itemValues),
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
                "capture_limit_bytes": .int(Self.maxOSCQueryNamespaceBytes),
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
                "provenance_class": .string("OBSERVED"),
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
            "provenance_class": .string(capture == "OBSERVED" ? "OBSERVED" : "UNSUPPORTED"),
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
            "provenance_class": .string("UNSUPPORTED"),
            "capture_status": .string("UNSUPPORTED"),
            "portability": .string("DESCRIPTIVE_ONLY"),
            "notes": .string("The declared launch target is not a safely inspectable local absolute path or file URL."),
        ]
    }

    private func applicationCaptureItem(_ application: URL?) -> [String: JSONValue] {
        var item: [String: JSONValue] = [
            "key": .string("vdmx-application"),
            "name": .string("VDMX application"),
            "kind": .string("OTHER"),
            "provenance": .string("ADAPTER_OBSERVATION"),
            "provenance_class": .string("OBSERVED"),
            "capture_status": .string(application == nil ? "MISSING" : "OBSERVED"),
            "portability": .string("DESCRIPTIVE_ONLY"),
            "notes": .string(application == nil
                ? "VDMX application bundle was not found at a safe known location."
                : "VDMX application bundle was found; application installation remains destination-specific."),
        ]
        guard let application else { return item }

        var metadata: [String: JSONValue] = [
            "bundle_path": .string(application.path),
        ]
        let infoURL = application
            .appendingPathComponent("Contents", isDirectory: true)
            .appendingPathComponent("Info.plist", isDirectory: false)
        if safeExistingURL(infoURL) != nil,
           let data = try? Data(contentsOf: infoURL),
           let plist = try? PropertyListSerialization.propertyList(
                from: data,
                options: [],
                format: nil
           ) as? [String: Any] {
            if let version = plist["CFBundleShortVersionString"] as? String,
               !version.isEmpty {
                metadata["version"] = .string(version)
            }
            if let build = plist["CFBundleVersion"] as? String,
               !build.isEmpty {
                metadata["build"] = .string(build)
            }
            if let bundleID = plist["CFBundleIdentifier"] as? String,
               !bundleID.isEmpty {
                metadata["bundle_identifier"] = .string(bundleID)
            }
        }
        item["metadata"] = .object(metadata)
        return item
    }

    private func itemHasStatus(
        _ items: [[String: JSONValue]],
        key: String,
        status: String
    ) -> Bool {
        items.contains { item in
            guard case .string(let itemKey)? = item["key"],
                  case .string(let itemStatus)? = item["capture_status"]
            else { return false }
            return itemKey == key && itemStatus == status
        }
    }

    private func rebuildPlan(
        applicationPresent: Bool,
        savedLaunchObserved: Bool,
        oscQueryObserved: Bool
    ) -> [JSONValue] {
        var steps: [JSONValue] = [
            .object([
                "step": .int(1),
                "action": .string("OPEN_VDMX_APPLICATION"),
                "status": .string(applicationPresent ? "SUPPORTED" : "MISSING"),
                "provenance_class": .string("OBSERVED"),
                "notes": .string("Launch only the observed VDMX application; do not infer a different edition or version."),
            ]),
            .object([
                "step": .int(2),
                "action": .string(savedLaunchObserved ? "OPEN_DECLARED_WORKSPACE" : "CREATE_UNSAVED_WORKSPACE"),
                "status": .string(savedLaunchObserved ? "REFERENCE_ONLY" : "MANUAL"),
                "provenance_class": .string(savedLaunchObserved ? "REFERENCE_ONLY" : "USER_DECLARED"),
                "notes": .string(savedLaunchObserved
                    ? "Use the declared saved workspace reference when it is available; snapshot metadata does not replace its bytes."
                    : "No safe saved workspace was observed. Recreate the unsaved/Demo workspace manually."),
            ]),
        ]

        steps.append(.object([
            "step": .int(3),
            "action": .string("VERIFY_PUBLISHED_OSCQUERY_NAMESPACE"),
            "status": .string(oscQueryObserved ? "OBSERVED" : "MANUAL"),
            "provenance_class": .string(oscQueryObserved ? "OBSERVED" : "UNSUPPORTED"),
            "notes": .string("Compare exact published OSC paths/types/ranges against the captured namespace before any state restore."),
        ]))
        steps.append(.object([
            "step": .int(4),
            "action": .string("RESTORE_STATEFUL_PUBLISHED_VALUES"),
            "status": .string("NOT_APPLIED"),
            "provenance_class": .string("OBSERVED"),
            "notes": .string("Only stateful controls may be considered later. Event/button/trigger controls are never replayed by this capture operation."),
        ]))
        steps.append(.object([
            "step": .int(5),
            "action": .string("REVIEW_UNSUPPORTED_INTERNAL_STATE"),
            "status": .string("MANUAL"),
            "provenance_class": .string("UNSUPPORTED"),
            "notes": .string("Unpublished layers, FX graphs, plugin internals and media-bin organization remain manual/unsupported unless a real saved VDMX project is available."),
        ]))
        return steps
    }

    private func reconstructionFingerprint(
        items: [JSONValue],
        rebuildPlan: [JSONValue]
    ) -> String {
        let value = JSONValue.object([
            "items": .array(items),
            "rebuild_plan": .array(rebuildPlan),
        ])
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let data = (try? encoder.encode(value)) ?? Data()
        return StageCoreSHA256.hexDigest(data)
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

private enum VDMXRestoreError: Error {
    case invalidEndpoint
    case invalidNamespace
    case invalidHostInfo
    case unsupportedType
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

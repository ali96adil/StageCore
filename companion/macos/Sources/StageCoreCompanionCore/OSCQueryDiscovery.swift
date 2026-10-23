#if os(macOS)
import Foundation

/// Read-only OSCQuery discovery. This never sends OSC messages or changes VDMX state.
public struct OSCQueryDiscoveryClient: Sendable {
    public struct HostInfo: Equatable, Sendable {
        public let name: String
        public let oscPort: Int
        public let transport: String
    }

    public struct Control: Equatable, Sendable {
        public let path: String
        public let description: String
        public let type: String
        public let minimum: Double?
        public let maximum: Double?
        public let access: Int?
    }

    public struct Result: Equatable, Sendable {
        public let host: HostInfo
        public let controls: [Control]
    }

    public enum DiscoveryError: Error {
        case invalidEndpoint
        case invalidResponse
        case unsupportedTransport
        case excessiveResponse
    }

    private let maxBytes = 1_048_576
    private let maxControls = 512

    public init() {}

    public func discover(at endpoint: URL) async throws -> Result {
        guard let scheme = endpoint.scheme?.lowercased(), ["http", "https"].contains(scheme),
              endpoint.user == nil, endpoint.password == nil,
              let host = endpoint.host?.lowercased(),
              ["localhost", "127.0.0.1", "::1"].contains(host),
              endpoint.port != nil, endpoint.query == nil, endpoint.fragment == nil,
              endpoint.path.isEmpty || endpoint.path == "/" else {
            throw DiscoveryError.invalidEndpoint
        }
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 2
        config.timeoutIntervalForResource = 3
        config.httpMaximumConnectionsPerHost = 1
        // Redirects must never escape the explicitly permitted loopback endpoint.
        let redirectBlocker = OSCQueryRedirectBlocker()
        let session = URLSession(configuration: config, delegate: redirectBlocker, delegateQueue: nil)
        defer { session.invalidateAndCancel() }

        let info = try await getJSON(endpoint: endpoint, query: "HOST_INFO", session: session)
        let tree = try await getJSON(endpoint: endpoint, query: nil, session: session)
        return try parse(hostInfo: info, tree: tree)
    }

    // Internal pure parser allows deterministic fixture tests without controlling live VDMX.
    func parse(hostInfo: [String: Any], tree: [String: Any]) throws -> Result {
        guard let name = hostInfo["NAME"] as? String,
              let port = hostInfo["OSC_PORT"] as? Int, (1...65535).contains(port),
              let transport = hostInfo["OSC_TRANSPORT"] as? String else {
            throw DiscoveryError.invalidResponse
        }
        guard transport.uppercased() == "UDP" else { throw DiscoveryError.unsupportedTransport }
        var controls: [Control] = []
        try collect(tree, depth: 0, into: &controls)
        return Result(host: HostInfo(name: name, oscPort: port, transport: transport), controls: controls)
    }

    private func getJSON(endpoint: URL, query: String?, session: URLSession) async throws -> [String: Any] {
        var url = endpoint
        if let query {
            var components = URLComponents(url: endpoint, resolvingAgainstBaseURL: false)
            components?.percentEncodedQuery = query
            guard let resolved = components?.url else { throw DiscoveryError.invalidEndpoint }
            url = resolved
        }
        var request = URLRequest(url: url)
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200,
              http.url?.host?.lowercased() == endpoint.host?.lowercased(),
              http.url?.port == endpoint.port,
              http.url?.scheme?.lowercased() == endpoint.scheme?.lowercased() else {
            throw DiscoveryError.invalidResponse
        }
        guard data.count <= maxBytes else { throw DiscoveryError.excessiveResponse }
        guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw DiscoveryError.invalidResponse
        }
        return object
    }

    private func collect(_ node: [String: Any], depth: Int, into controls: inout [Control]) throws {
        guard depth < 32, controls.count < maxControls else { throw DiscoveryError.excessiveResponse }
        if let path = node["FULL_PATH"] as? String,
           let type = node["TYPE"] as? String,
           path.hasPrefix("/"), !type.isEmpty {
            let range = (node["RANGE"] as? [[String: Any]])?.first
            let min = (range?["MIN"] as? NSNumber)?.doubleValue
            let max = (range?["MAX"] as? NSNumber)?.doubleValue
            controls.append(Control(
                path: path,
                description: node["DESCRIPTION"] as? String ?? "",
                type: type,
                minimum: min,
                maximum: max,
                access: (node["ACCESS"] as? NSNumber)?.intValue
            ))
        }
        if let children = node["CONTENTS"] as? [String: [String: Any]] {
            for key in children.keys.sorted() {
                if let child = children[key] {
                    try collect(child, depth: depth + 1, into: &controls)
                }
            }
        }
    }
}

/// Never follow an HTTP redirect, including redirects to another loopback port.
private final class OSCQueryRedirectBlocker: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    func urlSession(_ session: URLSession, task: URLSessionTask,
                    willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest,
                    completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}
#endif

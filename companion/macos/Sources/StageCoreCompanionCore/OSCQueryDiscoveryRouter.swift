#if os(macOS)
import Foundation

/// Separate, authenticated read-only request. Never uses cue execution authority.
public struct OSCQueryDiscoveryRouter: Sendable {
    public init() {}

    public func handle(_ data: Data, authenticated: Bool) async throws -> Data? {
        struct Header: Decodable {
            let type: String
        }
        let decoder = JSONDecoder()
        guard try decoder.decode(Header.self, from: data).type == "oscquery.discover.request" else {
            return nil
        }

        struct Request: Decodable {
            let schemaVersion: Int
            let requestID: String
            let endpoint: String

            enum CodingKeys: String, CodingKey {
                case schemaVersion = "schema_version"
                case requestID = "request_id"
                case endpoint
            }
        }
        struct Control: Encodable {
            let path: String
            let description: String
            let type: String
            let minimum: Double?
            let maximum: Double?
            let access: Int?
        }
        struct Response: Encodable {
            let type = "oscquery.discover.result"
            let schemaVersion = 1
            let requestID: String
            let status: String
            let errorCode: String?
            let name: String?
            let oscPort: Int?
            let controls: [Control]?

            enum CodingKeys: String, CodingKey {
                case type
                case schemaVersion = "schema_version"
                case requestID = "request_id"
                case status
                case errorCode = "error_code"
                case name
                case oscPort = "osc_port"
                case controls
            }
        }
        let request = try decoder.decode(Request.self, from: data)
        let encoder = JSONEncoder()
        func failure(_ code: String) throws -> Data {
            try encoder.encode(Response(requestID: request.requestID, status: "FAILED", errorCode: code,
                                        name: nil, oscPort: nil, controls: nil))
        }
        guard request.schemaVersion == 1, UUID(uuidString: request.requestID) != nil else {
            return try failure("OSCQUERY_REQUEST_INVALID")
        }
        guard authenticated else { return try failure("SESSION_UNAUTHENTICATED") }
        guard let url = URL(string: request.endpoint) else { return try failure("OSCQUERY_ENDPOINT_INVALID") }
        do {
            let result = try await OSCQueryDiscoveryClient().discover(at: url)
            // RuntimeChannel messages are capped at 64 KiB. Fail closed instead
            // of disconnecting the authenticated Companion with an oversized reply.
            let response = try encoder.encode(Response(
                requestID: request.requestID, status: "COMPLETED", errorCode: nil,
                name: result.host.name, oscPort: result.host.oscPort,
                controls: result.controls.map {
                    Control(path: $0.path, description: $0.description, type: $0.type,
                            minimum: $0.minimum, maximum: $0.maximum, access: $0.access)
                }
            ))
            guard response.count <= 60 * 1024 else {
                return try failure("OSCQUERY_RESULT_TOO_LARGE")
            }
            return response
        } catch OSCQueryDiscoveryClient.DiscoveryError.invalidEndpoint {
            return try failure("OSCQUERY_ENDPOINT_INVALID")
        } catch {
            return try failure("OSCQUERY_DISCOVERY_FAILED")
        }
    }
}
#endif

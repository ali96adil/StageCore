#if os(macOS)
import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class OSCQueryDiscoveryRouterTests: XCTestCase {
    private func request(_ url: String, id: String = "223e4567-e89b-12d3-a456-426614174000") throws -> Data {
        try JSONSerialization.data(withJSONObject: [
            "type": "oscquery.discover.request",
            "schema_version": 1,
            "request_id": id,
            "endpoint": url
        ])
    }

    func testDoesNotConsumeUnrelatedMessages() async throws {
        let data = Data(#"{"type":"inspection.request"}"#.utf8)
        let response = try await OSCQueryDiscoveryRouter().handle(data, authenticated: true)
        XCTAssertNil(response)
    }

    func testUnauthenticatedRequestIsRejectedWithoutNetworking() async throws {
        let data = try request("http://127.0.0.1:2345/")
        let response = try XCTUnwrap(await OSCQueryDiscoveryRouter().handle(data, authenticated: false))
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: response) as? [String: Any])
        XCTAssertEqual(object["status"] as? String, "FAILED")
        XCTAssertEqual(object["error_code"] as? String, "SESSION_UNAUTHENTICATED")
    }

    func testNetworkTargetsAreRejected() async throws {
        let data = try request("http://192.168.3.100:2345/")
        let response = try XCTUnwrap(await OSCQueryDiscoveryRouter().handle(data, authenticated: true))
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: response) as? [String: Any])
        XCTAssertEqual(object["error_code"] as? String, "OSCQUERY_ENDPOINT_INVALID")
    }
}
#endif

#if os(macOS)
import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class OSCQueryDiscoveryTests: XCTestCase {
    func testRejectsNonLoopbackEndpointBeforeNetworkAccess() async {
        let client = OSCQueryDiscoveryClient()
        for raw in [
            "http://example.com:2345/",
            "http://192.168.3.10:2345/",
            "file:///tmp/oscquery",
            "http://127.0.0.1:2345/other",
            "http://user:password@localhost:2345/",
            "http://localhost:2345/?HOST_INFO",
            "http://localhost/"
        ] {
            guard let url = URL(string: raw) else {
                XCTFail("Malformed test fixture: \(raw)")
                continue
            }
            do {
                _ = try await client.discover(at: url)
                XCTFail("Expected endpoint rejection: \(raw)")
            } catch OSCQueryDiscoveryClient.DiscoveryError.invalidEndpoint {
                // Expected.
            } catch {
                XCTFail("Wrong rejection for \(raw): \(error)")
            }
        }
    }
}
#endif

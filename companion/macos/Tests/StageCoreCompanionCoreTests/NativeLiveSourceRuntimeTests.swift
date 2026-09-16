#if os(macOS)
import XCTest
@testable import StageCoreCompanionCore

final class NativeLiveSourceRuntimeTests: XCTestCase {
    func testNetworkStreamRejectsUnsupportedSchemes() async {
        for endpoint in [
            "file:///tmp/not-a-network-stream.mov",
            "rtsp://example.invalid/live",
        ] {
            let runtime = NativeLiveSourceRuntime()
            let descriptor = LiveSourceRuntimeDescriptor(
                sourceID: "network-unsupported",
                sourceClass: .networkStream,
                endpointRef: endpoint
            )

            do {
                try await runtime.open(descriptor)
                XCTFail("expected unsupported network endpoint to be rejected: \(endpoint)")
            } catch let failure as LiveSourceRuntimeFailure {
                XCTAssertEqual(failure.code, "LIVE_SOURCE_ENDPOINT_INVALID")
            } catch {
                XCTFail("unexpected error: \(error)")
            }

            let snapshot = await runtime.snapshot()
            XCTAssertTrue(snapshot.openSourceIDs.isEmpty)
            XCTAssertTrue(snapshot.routes.isEmpty)
            XCTAssertNil(snapshot.selectedSourceID)
        }
    }
}
#endif

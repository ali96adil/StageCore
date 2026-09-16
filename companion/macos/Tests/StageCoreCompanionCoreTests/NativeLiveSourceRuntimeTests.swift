#if os(macOS)
import XCTest
@testable import StageCoreCompanionCore

final class NativeLiveSourceRuntimeTests: XCTestCase {
    func testNetworkStreamRejectsLocalFileEndpoint() async {
        let runtime = NativeLiveSourceRuntime()
        let descriptor = LiveSourceRuntimeDescriptor(
            sourceID: "network-local-file",
            sourceClass: .networkStream,
            endpointRef: "file:///tmp/not-a-network-stream.mov"
        )

        do {
            try await runtime.open(descriptor)
            XCTFail("expected local file endpoint to be rejected")
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
#endif

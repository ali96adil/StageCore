import XCTest

@testable import StageCoreCompanionCore

final class WebSocketHeartbeatTests: XCTestCase {
    func testHeartbeatRepeatsUntilTransportStopsIt() async {
        let counter = HeartbeatCounter()

        do {
            try await runCompanionHeartbeat(every: .milliseconds(1)) {
                let count = await counter.increment()
                if count == 2 {
                    throw HeartbeatStop.done
                }
            }
            XCTFail("heartbeat loop unexpectedly returned")
        } catch HeartbeatStop.done {
            // The transport controls loop termination.
        } catch {
            XCTFail("unexpected heartbeat error: \(error)")
        }

        let count = await counter.value
        XCTAssertEqual(count, 2)
    }
}

private enum HeartbeatStop: Error {
    case done
}

private actor HeartbeatCounter {
    private(set) var value = 0

    func increment() -> Int {
        value += 1
        return value
    }
}

import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class TransportPolicyTests: XCTestCase {
    func testProductionRequiresWSS() throws {
        XCTAssertThrowsError(
            try CompanionTransportPolicy.validate(
                url: XCTUnwrap(URL(string: "ws://127.0.0.1:18083/companion")),
                policy: .production
            )
        ) { error in
            XCTAssertEqual(error as? CompanionTransportError, .insecureTransportNotAllowed)
        }

        XCTAssertNoThrow(
            try CompanionTransportPolicy.validate(
                url: XCTUnwrap(URL(string: "wss://stagecore.local/companion")),
                policy: .production
            )
        )
    }

    func testInsecureTestingTransportIsLoopbackOnly() throws {
        XCTAssertNoThrow(
            try CompanionTransportPolicy.validate(
                url: XCTUnwrap(URL(string: "ws://127.0.0.1:18083/companion")),
                policy: .allowInsecureLoopbackForTesting
            )
        )
        XCTAssertNoThrow(
            try CompanionTransportPolicy.validate(
                url: XCTUnwrap(URL(string: "ws://localhost:18083/companion")),
                policy: .allowInsecureLoopbackForTesting
            )
        )
        XCTAssertThrowsError(
            try CompanionTransportPolicy.validate(
                url: XCTUnwrap(URL(string: "ws://192.168.1.50:18083/companion")),
                policy: .allowInsecureLoopbackForTesting
            )
        ) { error in
            XCTAssertEqual(error as? CompanionTransportError, .invalidLoopbackHost)
        }
    }
}

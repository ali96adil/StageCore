import XCTest
@testable import StageCoreCompanionCore

final class VisualProjectionValidationTests: XCTestCase {
    func testIdentityAndKeystoneMappingsAreValid() {
        XCTAssertTrue(VisualQuadState.identity.isValidProjection)

        let keystone = VisualQuadState(
            topLeft: .init(x: 0.03, y: 0.04),
            topRight: .init(x: 0.98, y: 0.01),
            bottomRight: .init(x: 0.94, y: 0.97),
            bottomLeft: .init(x: 0.05, y: 0.99)
        )
        XCTAssertTrue(keystone.isValidProjection)
    }

    func testDegenerateMappingIsRejected() {
        let mapping = VisualQuadState(
            topLeft: .init(x: 0, y: 0),
            topRight: .init(x: 1, y: 0),
            bottomRight: .init(x: 1, y: 0),
            bottomLeft: .init(x: 0, y: 0)
        )
        XCTAssertFalse(mapping.isValidProjection)
    }

    func testSelfCrossingMappingIsRejected() {
        let mapping = VisualQuadState(
            topLeft: .init(x: 0, y: 0),
            topRight: .init(x: 1, y: 1),
            bottomRight: .init(x: 1, y: 0),
            bottomLeft: .init(x: 0, y: 1)
        )
        XCTAssertFalse(mapping.isValidProjection)
    }

    func testConcaveMappingIsRejected() {
        let mapping = VisualQuadState(
            topLeft: .init(x: 0, y: 0),
            topRight: .init(x: 1, y: 0),
            bottomRight: .init(x: 0.25, y: 0.25),
            bottomLeft: .init(x: 0, y: 1)
        )
        XCTAssertFalse(mapping.isValidProjection)
    }

    func testOutOfBoundsMappingIsRejected() {
        let mapping = VisualQuadState(
            topLeft: .init(x: -4.01, y: 0),
            topRight: .init(x: 1, y: 0),
            bottomRight: .init(x: 1, y: 1),
            bottomLeft: .init(x: 0, y: 1)
        )
        XCTAssertFalse(mapping.isValidProjection)
    }
}

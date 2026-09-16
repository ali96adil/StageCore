import Foundation

public struct VisualPointState: Sendable, Equatable {
    public var x: Double
    public var y: Double

    public init(x: Double, y: Double) {
        self.x = x
        self.y = y
    }
}

public struct VisualQuadState: Sendable, Equatable {
    public var topLeft: VisualPointState
    public var topRight: VisualPointState
    public var bottomRight: VisualPointState
    public var bottomLeft: VisualPointState

    public init(
        topLeft: VisualPointState,
        topRight: VisualPointState,
        bottomRight: VisualPointState,
        bottomLeft: VisualPointState
    ) {
        self.topLeft = topLeft
        self.topRight = topRight
        self.bottomRight = bottomRight
        self.bottomLeft = bottomLeft
    }

    public static let identity = VisualQuadState(
        topLeft: VisualPointState(x: 0, y: 0),
        topRight: VisualPointState(x: 1, y: 0),
        bottomRight: VisualPointState(x: 1, y: 1),
        bottomLeft: VisualPointState(x: 0, y: 1)
    )

    public var signedArea: Double {
        let points = [topLeft, topRight, bottomRight, bottomLeft]
        var sum = 0.0
        for index in points.indices {
            let next = points[(index + 1) % points.count]
            sum += points[index].x * next.y - next.x * points[index].y
        }
        return sum / 2
    }

    public var isValidProjection: Bool {
        let points = [topLeft, topRight, bottomRight, bottomLeft]
        guard points.allSatisfy({
            $0.x.isFinite && $0.y.isFinite &&
            $0.x >= -4 && $0.x <= 4 && $0.y >= -4 && $0.y <= 4
        }) else {
            return false
        }
        return abs(signedArea) > 0.000_001
    }
}

public struct VisualOutputState: Sendable, Equatable {
    public var outputID: String
    public var width: Double
    public var height: Double
    public var mapping: VisualQuadState

    public init(
        outputID: String,
        width: Double,
        height: Double,
        mapping: VisualQuadState = .identity
    ) {
        self.outputID = outputID
        self.width = width
        self.height = height
        self.mapping = mapping
    }
}

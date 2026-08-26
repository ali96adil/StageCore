// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "StageCoreCompanion",
    platforms: [
        .macOS(.v14),
    ],
    products: [
        .library(
            name: "StageCoreCompanionCore",
            targets: ["StageCoreCompanionCore"]
        ),
    ],
    targets: [
        .target(name: "StageCoreCompanionCore"),
        .testTarget(
            name: "StageCoreCompanionCoreTests",
            dependencies: ["StageCoreCompanionCore"]
        ),
    ]
)

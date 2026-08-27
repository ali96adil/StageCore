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
        .executable(
            name: "stagecore-companion",
            targets: ["StageCoreCompanion"]
        ),
    ],
    targets: [
        .target(
            name: "StageCoreCompanionCore",
            linkerSettings: [
                .linkedFramework("Security", .when(platforms: [.macOS])),
                .linkedFramework("CoreMIDI", .when(platforms: [.macOS])),
            ]
        ),
        .executableTarget(
            name: "StageCoreCompanion",
            dependencies: ["StageCoreCompanionCore"]
        ),
        .testTarget(
            name: "StageCoreCompanionCoreTests",
            dependencies: ["StageCoreCompanionCore"]
        ),
    ]
)

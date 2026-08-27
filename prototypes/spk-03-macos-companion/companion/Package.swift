// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "StageCoreCompanionSpike",
    products: [
        .library(name: "StageCoreCompanionCore", targets: ["StageCoreCompanionCore"]),
        .executable(name: "stagecore-companion-cli", targets: ["stagecore-companion-cli"]),
    ],
    targets: [
        .target(name: "StageCoreCompanionCore"),
        .executableTarget(name: "stagecore-companion-cli", dependencies: ["StageCoreCompanionCore"]),
        .testTarget(name: "StageCoreCompanionCoreTests", dependencies: ["StageCoreCompanionCore"]),
    ]
)

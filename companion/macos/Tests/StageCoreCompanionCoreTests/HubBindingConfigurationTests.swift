import Foundation
import Testing
@testable import StageCoreCompanionCore

@Test("legacy manual Companion configuration decodes without Hub binding")
func legacyConfigurationRemainsCompatible() throws {
    let json = #"{"hubAPIBaseURL":"https://stagecore.example/","hubRuntimeURL":"wss://stagecore.example/api/v1/companion/runtime","displayName":"Video Mac","agentVersion":"0.1.0","configHash":""}"#
    let configuration = try JSONDecoder().decode(CompanionAppConfiguration.self, from: Data(json.utf8))
    #expect(configuration.hubAPIBaseURL.absoluteString == "https://stagecore.example/")
    #expect(configuration.hubRuntimeURL.absoluteString == "wss://stagecore.example/api/v1/companion/runtime")
    #expect(configuration.hubBinding == nil)
}

@Test("remembered Hub binding persists with Companion configuration")
func hubBindingRoundTrips() throws {
    let configuration = CompanionAppConfiguration(
        hubAPIBaseURL: URL(string: "https://stagecore-abcd.local:7841/")!,
        hubRuntimeURL: URL(string: "wss://stagecore-abcd.local:7841/api/v1/companion/runtime")!,
        displayName: "Video Mac",
        hubBinding: CompanionHubBinding(
            hubID: "01a045ef-1d7d-7b9b-8bb3-c0daa63fc19d",
            fingerprint: "SHA256:hub",
            tlsCertificateSHA256: String(repeating: "a", count: 64)
        )
    )
    let encoded = try JSONEncoder().encode(configuration)
    let decoded = try JSONDecoder().decode(CompanionAppConfiguration.self, from: encoded)
    #expect(decoded == configuration)
    #expect(decoded.hubBinding?.hubID == "01a045ef-1d7d-7b9b-8bb3-c0daa63fc19d")
}

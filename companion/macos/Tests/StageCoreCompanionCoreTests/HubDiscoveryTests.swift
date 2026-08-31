import Foundation
import Testing
@testable import StageCoreCompanionCore

private let validTXT: [String: String] = [
    "v": "1",
    "hub_id": "01a045ef-1d7d-7b9b-8bb3-c0daa63fc19d",
    "name": "Main Stage Hub",
    "hub_fp": "SHA256:hub-fingerprint",
    "tls_sha256": String(repeating: "a", count: 64),
    "host": "stagecore-01a045ef1d7d.local",
    "port": "7841",
    "api_path": "/",
    "runtime_path": "/api/v1/companion/runtime",
]

@Test("valid Bonjour TXT creates secure Hub URLs")
func validBonjourTXTCreatesSecureHubURLs() throws {
    let hub = try DiscoveredHub(txt: validTXT)
    #expect(hub.displayName == "Main Stage Hub")
    #expect(hub.apiBaseURL.absoluteString == "https://stagecore-01a045ef1d7d.local:7841/")
    #expect(hub.runtimeURL.absoluteString == "wss://stagecore-01a045ef1d7d.local:7841/api/v1/companion/runtime")
    #expect(hub.binding.matches(hub))
}

@Test("untrusted Bonjour TXT rejects malformed endpoint fields")
func malformedBonjourTXTIsRejected() {
    let mutations: [(String, String)] = [
        ("v", "2"),
        ("hub_id", "not-a-uuid"),
        ("tls_sha256", "bad"),
        ("host", "example.com"),
        ("host", "stagecore.local/evil"),
        ("port", "0"),
        ("port", "70000"),
        ("api_path", "https://evil.invalid/"),
        ("runtime_path", "/other"),
    ]
    for (key, value) in mutations {
        var txt = validTXT
        txt[key] = value
        #expect(throws: HubDiscoveryError.malformedAdvertisement) {
            _ = try DiscoveredHub(txt: txt)
        }
    }
}

@Test("first pairing never guesses when multiple Hubs are present")
func firstPairSelectionIsExplicitWhenAmbiguous() throws {
    let first = try DiscoveredHub(txt: validTXT)
    var secondTXT = validTXT
    secondTXT["hub_id"] = "01a047ff-b945-79bb-a1f0-9bb528b7dabd"
    secondTXT["name"] = "Main Stage Hub"
    secondTXT["host"] = "stagecore-01a047ffb945.local"
    secondTXT["tls_sha256"] = String(repeating: "b", count: 64)
    let second = try DiscoveredHub(txt: secondTXT)

    #expect(throws: HubDiscoveryError.multipleHubsFound) {
        _ = try HubDiscoverySelection.firstPairCandidate(from: [first, second])
    }
}

@Test("remembered Hub matching uses identity not display name")
func rememberedHubCannotBeSubstitutedBySameName() throws {
    let expected = try DiscoveredHub(txt: validTXT)
    var fakeTXT = validTXT
    fakeTXT["hub_id"] = "01a047ff-b945-79bb-a1f0-9bb528b7dabd"
    fakeTXT["host"] = "stagecore-01a047ffb945.local"
    fakeTXT["tls_sha256"] = String(repeating: "c", count: 64)
    let fake = try DiscoveredHub(txt: fakeTXT)

    let selected = try HubDiscoverySelection.remembered(expected.binding, from: [fake, expected])
    #expect(selected == expected)
    #expect(throws: HubDiscoveryError.rememberedHubUnavailable) {
        _ = try HubDiscoverySelection.remembered(expected.binding, from: [fake])
    }
}

@Test("public Hub identity must match discovered identity")
func publicHubIdentityMustMatchDiscovery() throws {
    let hub = try DiscoveredHub(txt: validTXT)
    let valid = PublicHubIdentity(
        schemaVersion: 1,
        hubID: hub.hubID,
        displayName: hub.displayName,
        fingerprint: hub.fingerprint,
        bootstrapState: "CLAIMED"
    )
    try HubIdentityVerifier.validate(valid, matches: hub)

    let mismatch = PublicHubIdentity(
        schemaVersion: 1,
        hubID: hub.hubID,
        displayName: hub.displayName,
        fingerprint: "SHA256:different",
        bootstrapState: "CLAIMED"
    )
    #expect(throws: HubDiscoveryError.hubIdentityMismatch) {
        try HubIdentityVerifier.validate(mismatch, matches: hub)
    }
}

@Test("TLS certificate pins are strict lowercase-or-uppercase hex")
func certificatePinValidation() {
    #expect(HubTLS.isValidCertificateSHA256(String(repeating: "A", count: 64)))
    #expect(!HubTLS.isValidCertificateSHA256(String(repeating: "g", count: 64)))
    #expect(!HubTLS.isValidCertificateSHA256("abcd"))
}

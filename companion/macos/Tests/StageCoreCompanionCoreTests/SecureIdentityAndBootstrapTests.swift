import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class SecureIdentityAndBootstrapTests: XCTestCase {
    func testFirstLaunchCreatesStableIdentityAndMetadataChangesDoNotReplaceIt() async throws {
        let store = MemorySecureIdentityStore()
        let first = try store.loadOrCreateIdentity()
        let second = try store.loadOrCreateIdentity()
        XCTAssertEqual(first, second)
        XCTAssertTrue(CompanionIdentity.isCanonicalID(first.companionID))

        let firstBootstrap = try CompanionBootstrap(
            configuration: configuration(displayName: "Video Mac A"),
            identityStore: store,
            hostname: "video-a.local"
        )
        let renamedBootstrap = try CompanionBootstrap(
            configuration: configuration(displayName: "Renamed Video Mac"),
            identityStore: store,
            hostname: "video-b.local"
        )
        let firstStatus = await firstBootstrap.status()
        XCTAssertEqual(firstStatus.companionID, first.companionID)
        XCTAssertEqual(firstStatus.readiness, .unknown)
        let renamedStatus = await renamedBootstrap.status()
        XCTAssertEqual(renamedStatus.companionID, first.companionID)
        XCTAssertEqual(renamedStatus.displayName, "Renamed Video Mac")
    }

    func testConfiguredOSCIsAdvertisedAndMissingOSCIsNotFabricated() async throws {
        let withoutOSC = try CompanionBootstrap(
            configuration: configuration(displayName: "Video Mac"),
            identityStore: MemorySecureIdentityStore()
        )
        let withoutStatus = await withoutOSC.status()
        #if os(macOS)
        XCTAssertEqual(withoutStatus.capabilities, ["local.echo", "midi.send"])
        #else
        XCTAssertEqual(withoutStatus.capabilities, ["local.echo"])
        #endif

        let withOSC = try CompanionBootstrap(
            configuration: configuration(
                displayName: "Video Mac OSC",
                oscEndpoint: OSCEndpoint(host: "127.0.0.1", port: 9000)
            ),
            identityStore: MemorySecureIdentityStore()
        )
        let withStatus = await withOSC.status()
        #if os(macOS)
        XCTAssertEqual(withStatus.capabilities, ["local.echo", "midi.send", "osc.send"])
        #else
        XCTAssertEqual(withStatus.capabilities, ["local.echo", "osc.send"])
        #endif
    }

    func testNormalConfigurationContainsNoPrivateCredentialMaterial() throws {
        let configuration = configuration(displayName: "Video Mac")
        let data = try JSONEncoder().encode(configuration)
        let json = try XCTUnwrap(String(data: data, encoding: .utf8))
        XCTAssertFalse(json.localizedCaseInsensitiveContains("private"))
        XCTAssertFalse(json.localizedCaseInsensitiveContains("secret"))
        XCTAssertFalse(json.localizedCaseInsensitiveContains("token"))
        XCTAssertFalse(json.contains(MemorySecureIdentityStore.privateMaterialMarker))
    }

    func testUnauthenticatedSessionRejectsRuntimeWork() async throws {
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [LocalEchoExecutor()]
        )
        let request = CompanionExecutionRequest(
            executionID: "exec-unpaired",
            correlationID: nil,
            machineRoleID: "role-video-main",
            runtimeSnapshotID: "snapshot-1",
            capability: "local.echo",
            parameters: [:],
            timeoutMS: 100
        )
        let response = try await session.handle(JSONEncoder().encode(request))
        let data = try XCTUnwrap(response)
        let result = try JSONDecoder().decode(CompanionExecutionResult.self, from: data)
        XCTAssertEqual(result.status, .rejected)
        XCTAssertEqual(result.errorCode, "SESSION_UNAUTHENTICATED")
    }

    func testAuthenticationMessageMatchesHubContract() {
        let message = HubSecurityClient.authenticationMessage(
            companionID: "companion-1",
            challengeID: "challenge-1",
            nonceBase64: "bm9uY2U="
        )
        XCTAssertEqual(
            String(data: message, encoding: .utf8),
            "StageCore Companion Authentication v1\ncompanion-1\nchallenge-1\nbm9uY2U="
        )
    }

    private func configuration(
        displayName: String,
        oscEndpoint: OSCEndpoint? = nil
    ) -> CompanionAppConfiguration {
        CompanionAppConfiguration(
            hubAPIBaseURL: URL(string: "https://stagecore.local/")!,
            hubRuntimeURL: URL(string: "wss://stagecore.local/companion")!,
            displayName: displayName,
            oscEndpoint: oscEndpoint
        )
    }
}

private final class MemorySecureIdentityStore: SecureDeviceIdentityStore, @unchecked Sendable {
    static let privateMaterialMarker = "test-private-device-key-material"
    private let lock = NSLock()
    private var identity: SecureDeviceIdentity?

    func loadOrCreateIdentity() throws -> SecureDeviceIdentity {
        lock.withLock {
            if let identity { return identity }
            let created = SecureDeviceIdentity(
                companionID: CompanionIdentity.generateID(),
                publicKeyData: Data("test-public-key".utf8)
            )
            identity = created
            return created
        }
    }

    func signAuthenticationChallenge(_ message: Data) throws -> Data {
        Data((Self.privateMaterialMarker + message.base64EncodedString()).utf8)
    }
}

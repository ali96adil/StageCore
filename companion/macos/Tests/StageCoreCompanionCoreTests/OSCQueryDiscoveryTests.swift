#if os(macOS)
import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class OSCQueryDiscoveryTests: XCTestCase {
    func testRejectsNonLoopbackEndpointBeforeNetworkAccess() async {
        let client = OSCQueryDiscoveryClient()
        for raw in [
            "http://example.com:2345/",
            "http://192.168.3.10:2345/",
            "file:///tmp/oscquery",
            "http://127.0.0.1:2345/other",
            "http://user:password@localhost:2345/",
            "http://localhost:2345/?HOST_INFO",
            "http://localhost/"
        ] {
            guard let url = URL(string: raw) else {
                XCTFail("Malformed test fixture: \(raw)")
                continue
            }
            do {
                _ = try await client.discover(at: url)
                XCTFail("Expected endpoint rejection: \(raw)")
            } catch OSCQueryDiscoveryClient.DiscoveryError.invalidEndpoint {
                // Expected.
            } catch {
                XCTFail("Wrong rejection for \(raw): \(error)")
            }
        }
    }
    func testParsesVDMXHostAndPublishedSlider() throws {
        let info: [String: Any] = [
            "NAME": "VDMX6-OSCQuery", "OSC_PORT": 3546, "OSC_TRANSPORT": "UDP"
        ]
        let tree: [String: Any] = [
            "CONTENTS": [
                "OSCQUERY": [
                    "CONTENTS": [
                        "Control Surface": [
                            "CONTENTS": [
                                "Slider": [
                                    "FULL_PATH": "/OSCQUERY/Control Surface/Slider",
                                    "TYPE": "f",
                                    "DESCRIPTION": "Slider Slider",
                                    "RANGE": [["MIN": 0, "MAX": 1]]
                                ]
                            ]
                        ]
                    ]
                ]
            ]
        ]
        let result = try OSCQueryDiscoveryClient().parse(hostInfo: info, tree: tree)
        XCTAssertEqual(result.host.name, "VDMX6-OSCQuery")
        XCTAssertEqual(result.host.oscPort, 3546)
        XCTAssertEqual(result.host.transport, "UDP")
        XCTAssertEqual(result.controls.count, 1)
        XCTAssertEqual(result.controls[0].path, "/OSCQUERY/Control Surface/Slider")
        XCTAssertEqual(result.controls[0].type, "f")
        XCTAssertEqual(result.controls[0].minimum, 0)
        XCTAssertEqual(result.controls[0].maximum, 1)
    }

    func testRejectsMissingHostInfoAndUnsupportedTransport() {
        let client = OSCQueryDiscoveryClient()
        XCTAssertThrowsError(try client.parse(hostInfo: [:], tree: [:]))
        XCTAssertThrowsError(try client.parse(
            hostInfo: ["NAME": "VDMX", "OSC_PORT": 3546, "OSC_TRANSPORT": "TCP"],
            tree: [:]
        ))
    }

    func testRejectsExcessiveTreeDepth() {
        var node: [String: Any] = ["FULL_PATH": "/deep", "TYPE": "f"]
        for _ in 0..<34 { node = ["CONTENTS": ["child": node]] }
        XCTAssertThrowsError(try OSCQueryDiscoveryClient().parse(
            hostInfo: ["NAME": "VDMX", "OSC_PORT": 3546, "OSC_TRANSPORT": "UDP"],
            tree: node
        ))
    }

    /// Opt-in live test; never affects default CI or sends a UDP OSC command.
    /// Run with STAGECORE_OSCQUERY_LIVE_URL=http://127.0.0.1:2345/
    func testLiveOSCQueryDiscoveryWhenEnabled() async throws {
        guard let endpointText = ProcessInfo.processInfo.environment["STAGECORE_OSCQUERY_LIVE_URL"],
              !endpointText.isEmpty else {
            throw XCTSkip("Set STAGECORE_OSCQUERY_LIVE_URL to run the live VDMX discovery check")
        }
        let endpoint = try XCTUnwrap(URL(string: endpointText))
        let result = try await OSCQueryDiscoveryClient().discover(at: endpoint)
        XCTAssertFalse(result.host.name.isEmpty)
        XCTAssertTrue((1...65535).contains(result.host.oscPort))
        XCTAssertEqual(result.host.transport.uppercased(), "UDP")
        XCTAssertFalse(result.controls.isEmpty, "VDMX must publish at least one OSCQuery control")
        for control in result.controls {
            XCTAssertTrue(control.path.hasPrefix("/"))
            XCTAssertFalse(control.type.isEmpty)
        }
        print("OSCQuery: \\(result.host.name), UDP \\(result.host.oscPort), controls \\(result.controls.count)")
        for control in result.controls.prefix(20) {
            print("CONTROL \\(control.path) type=\\(control.type) range=\\(String(describing: control.minimum))...\\(String(describing: control.maximum))")
        }
    }

}
#endif

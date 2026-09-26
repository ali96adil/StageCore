import Foundation
import Testing
@testable import StageCoreCompanionCore

@Suite("Local OSC control")
struct LocalOSCControlTests {
    @Test("accepts /stagecore/go with no arguments")
    func acceptsNoArguments() {
        #expect(LocalOSCControlParser.isGo(packet(address: "/stagecore/go", typeTags: ",")))
    }

    @Test("accepts /stagecore/go integer one")
    func acceptsIntegerOne() {
        var payload = Data()
        var one = Int32(1).bigEndian
        withUnsafeBytes(of: &one) { payload.append(contentsOf: $0) }
        #expect(LocalOSCControlParser.isGo(packet(address: "/stagecore/go", typeTags: ",i", payload: payload)))
    }

    @Test("rejects other paths and values")
    func rejectsOtherInput() {
        #expect(!LocalOSCControlParser.isGo(packet(address: "/stagecore/next", typeTags: ",")))

        var payload = Data()
        var two = Int32(2).bigEndian
        withUnsafeBytes(of: &two) { payload.append(contentsOf: $0) }
        #expect(!LocalOSCControlParser.isGo(packet(address: "/stagecore/go", typeTags: ",i", payload: payload)))
    }

    private func packet(address: String, typeTags: String, payload: Data = Data()) -> Data {
        var result = Data()
        appendPadded(address, to: &result)
        appendPadded(typeTags, to: &result)
        result.append(payload)
        return result
    }

    private func appendPadded(_ value: String, to data: inout Data) {
        data.append(contentsOf: value.utf8)
        data.append(0)
        while data.count % 4 != 0 {
            data.append(0)
        }
    }
}

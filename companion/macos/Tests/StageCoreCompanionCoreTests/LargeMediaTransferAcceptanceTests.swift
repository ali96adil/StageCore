#if os(macOS)
import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class LargeMediaTransferAcceptanceTests: XCTestCase {
    func testTwoGiBInterruptedTransferResumesAndVerifies() async throws {
        let totalBytes: Int64 = 2 * 1024 * 1024 * 1024 + 1
        let contentHash = "b8030a8ab89280935633d8d991da3d9907c0f12e8b6fc3bfc515f4d440872b6e"
        let chunkBytes = MediaCacheSynchronizer.transferChunkBytes
        LargeTransferURLProtocol.state.reset(
            totalBytes: totalBytes,
            contentHash: contentHash,
            failOnRequestNumber: 3
        )

        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [LargeTransferURLProtocol.self]
        let urlSession = URLSession(configuration: configuration)
        defer { urlSession.invalidateAndCancel() }

        let cacheRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("stagecore-large-media-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: cacheRoot) }

        let synchronizer = try MediaCacheSynchronizer(
            apiBaseURL: URL(string: "http://127.0.0.1:7840")!,
            cacheRoot: cacheRoot,
            securityPolicy: .allowInsecureLoopbackForTesting,
            session: urlSession
        )
        let media = RequiredMedia(
            mediaAssetID: "asset-large",
            contentVersionID: "content-large",
            contentHash: contentHash,
            sizeBytes: totalBytes
        )

        let first = await synchronizer.synchronize(
            requiredMedia: [media],
            sessionToken: "acceptance-session"
        )
        guard case .failed = first else {
            XCTFail("interrupted transfer unexpectedly completed: \(first)")
            return
        }

        let partialURL = await synchronizer.partialObjectURL(for: contentHash)
        let partialSize = try fileSize(partialURL)
        XCTAssertEqual(partialSize, 2 * chunkBytes)
        let finalBeforeResume = await synchronizer.verifiedObjectURL(for: contentHash)
        XCTAssertFalse(FileManager.default.fileExists(atPath: finalBeforeResume.path))

        LargeTransferURLProtocol.state.disableFailure()
        let second = await synchronizer.synchronize(
            requiredMedia: [media],
            sessionToken: "acceptance-session"
        )
        XCTAssertEqual(second, .ready)

        let finalURL = await synchronizer.verifiedObjectURL(for: contentHash)
        XCTAssertEqual(try fileSize(finalURL), totalBytes)
        XCTAssertFalse(FileManager.default.fileExists(atPath: partialURL.path))

        let ranges = LargeTransferURLProtocol.state.recordedRanges()
        let resumeRange = "bytes=\(2 * chunkBytes)-\(3 * chunkBytes - 1)"
        XCTAssertGreaterThanOrEqual(ranges.filter { $0 == resumeRange }.count, 2)
        XCTAssertTrue(LargeTransferURLProtocol.state.allRequestsAuthenticated())
    }

    private func fileSize(_ url: URL) throws -> Int64 {
        let attributes = try FileManager.default.attributesOfItem(atPath: url.path)
        return (attributes[.size] as? NSNumber)?.int64Value ?? -1
    }
}

private final class LargeTransferProtocolState: @unchecked Sendable {
    private let lock = NSLock()
    private var totalBytes: Int64 = 0
    private var contentHash = ""
    private var failOnRequestNumber: Int?
    private var requestCount = 0
    private var ranges: [String] = []
    private var authenticated = true

    func reset(totalBytes: Int64, contentHash: String, failOnRequestNumber: Int?) {
        lock.lock()
        defer { lock.unlock() }
        self.totalBytes = totalBytes
        self.contentHash = contentHash
        self.failOnRequestNumber = failOnRequestNumber
        requestCount = 0
        ranges = []
        authenticated = true
    }

    func disableFailure() {
        lock.lock()
        failOnRequestNumber = nil
        lock.unlock()
    }

    func recordedRanges() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        return ranges
    }

    func allRequestsAuthenticated() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return authenticated
    }

    func plan(for request: URLRequest) -> LargeTransferPlan {
        lock.lock()
        defer { lock.unlock() }

        requestCount += 1
        if request.value(forHTTPHeaderField: "Authorization") != "StageCoreSession acceptance-session" {
            authenticated = false
        }
        let range = request.value(forHTTPHeaderField: "Range") ?? ""
        ranges.append(range)
        if failOnRequestNumber == requestCount {
            return .failure
        }
        guard let parsed = Self.parseRange(range),
              parsed.start >= 0,
              parsed.end >= parsed.start,
              parsed.end < totalBytes else {
            return .invalid
        }
        return .data(
            start: parsed.start,
            end: parsed.end,
            totalBytes: totalBytes,
            contentHash: contentHash
        )
    }

    private static func parseRange(_ value: String) -> (start: Int64, end: Int64)? {
        guard value.hasPrefix("bytes=") else { return nil }
        let fields = value.dropFirst("bytes=".count).split(separator: "-", maxSplits: 1)
        guard fields.count == 2,
              let start = Int64(fields[0]),
              let end = Int64(fields[1]) else {
            return nil
        }
        return (start, end)
    }
}

private enum LargeTransferPlan {
    case data(start: Int64, end: Int64, totalBytes: Int64, contentHash: String)
    case failure
    case invalid
}

private final class LargeTransferURLProtocol: URLProtocol {
    static let state = LargeTransferProtocolState()

    override class func canInit(with request: URLRequest) -> Bool {
        request.url?.host == "127.0.0.1"
    }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        switch Self.state.plan(for: request) {
        case .failure:
            client?.urlProtocol(self, didFailWithError: URLError(.networkConnectionLost))
        case .invalid:
            client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
        case .data(let start, let end, let totalBytes, let contentHash):
            let length = end - start + 1
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 206,
                httpVersion: "HTTP/1.1",
                headerFields: [
                    "Content-Range": "bytes \(start)-\(end)/\(totalBytes)",
                    "Content-Length": "\(length)",
                    "Accept-Ranges": "bytes",
                    "X-Content-SHA256": contentHash,
                ]
            )!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            var remaining = length
            let generatorChunk = 1024 * 1024
            let zeroChunk = Data(repeating: 0, count: generatorChunk)
            while remaining > 0 {
                let count = min(Int64(generatorChunk), remaining)
                if count == Int64(generatorChunk) {
                    client?.urlProtocol(self, didLoad: zeroChunk)
                } else {
                    client?.urlProtocol(self, didLoad: Data(repeating: 0, count: Int(count)))
                }
                remaining -= count
            }
            client?.urlProtocolDidFinishLoading(self)
        }
    }

    override func stopLoading() {}
}
#endif

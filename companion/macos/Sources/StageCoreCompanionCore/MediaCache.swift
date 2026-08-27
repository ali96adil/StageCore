#if os(macOS)
import Foundation
import CryptoKit

public enum MediaCacheError: Error, Equatable {
    case secureTransportRequired
    case invalidLoopbackHost
    case unsupportedChecksum(String)
    case invalidResponse
    case rangeNotHonored
    case sizeMismatch
    case checksumMismatch
}

public actor MediaCacheSynchronizer: CompanionMediaSynchronizer {
    private let apiBaseURL: URL
    private let cacheRoot: URL
    private let securityPolicy: CompanionTransportSecurityPolicy
    private let session: URLSession
    private let fileManager: FileManager

    public init(
        apiBaseURL: URL,
        cacheRoot: URL,
        securityPolicy: CompanionTransportSecurityPolicy = .production,
        session: URLSession = URLSession(configuration: .ephemeral),
        fileManager: FileManager = .default
    ) throws {
        try Self.validateAPIURL(apiBaseURL, policy: securityPolicy)
        self.apiBaseURL = apiBaseURL
        self.cacheRoot = cacheRoot
        self.securityPolicy = securityPolicy
        self.session = session
        self.fileManager = fileManager
        try Self.prepareDirectories(cacheRoot: cacheRoot, fileManager: fileManager)
    }

    public func synchronize(requiredMedia: [RequiredMedia], sessionToken: String) async -> MediaSyncResult {
        for item in requiredMedia where item.required {
            do {
                try Task.checkCancellation()
                try await synchronizeOne(item, sessionToken: sessionToken)
            } catch MediaCacheError.checksumMismatch {
                return .mismatch(item.contentHash)
            } catch MediaCacheError.sizeMismatch {
                return .mismatch(item.contentHash)
            } catch {
                return .failed(String(describing: error))
            }
        }
        return .ready
    }

    public func verifiedObjectURL(for contentHash: String) -> URL {
        cacheRoot
            .appendingPathComponent("objects", isDirectory: true)
            .appendingPathComponent("sha256", isDirectory: true)
            .appendingPathComponent(String(contentHash.prefix(2)), isDirectory: true)
            .appendingPathComponent(String(contentHash.dropFirst(2).prefix(2)), isDirectory: true)
            .appendingPathComponent(contentHash, isDirectory: false)
    }

    public func partialObjectURL(for contentHash: String) -> URL {
        cacheRoot
            .appendingPathComponent("staging", isDirectory: true)
            .appendingPathComponent(contentHash + ".part", isDirectory: false)
    }

    private func synchronizeOne(_ item: RequiredMedia, sessionToken: String) async throws {
        guard item.checksumAlgorithm.uppercased() == "SHA256" else {
            throw MediaCacheError.unsupportedChecksum(item.checksumAlgorithm)
        }
        guard item.contentHash.count == 64, item.sizeBytes >= 0 else {
            throw MediaCacheError.invalidResponse
        }

        let finalURL = verifiedObjectURL(for: item.contentHash)
        if fileManager.fileExists(atPath: finalURL.path) {
            let attributes = try fileManager.attributesOfItem(atPath: finalURL.path)
            let size = (attributes[.size] as? NSNumber)?.int64Value ?? -1
            guard size == item.sizeBytes else {
                throw MediaCacheError.sizeMismatch
            }
            guard try sha256Hex(of: finalURL) == item.contentHash.lowercased() else {
                throw MediaCacheError.checksumMismatch
            }
            return
        }

        let partURL = partialObjectURL(for: item.contentHash)
        var offset: Int64 = 0
        if fileManager.fileExists(atPath: partURL.path) {
            let attributes = try fileManager.attributesOfItem(atPath: partURL.path)
            offset = (attributes[.size] as? NSNumber)?.int64Value ?? 0
            if offset > item.sizeBytes {
                throw MediaCacheError.sizeMismatch
            }
        } else {
            fileManager.createFile(atPath: partURL.path, contents: nil)
        }

        if offset < item.sizeBytes {
            try await appendDownload(item, offset: offset, sessionToken: sessionToken, partURL: partURL)
        }

        let attributes = try fileManager.attributesOfItem(atPath: partURL.path)
        let completedSize = (attributes[.size] as? NSNumber)?.int64Value ?? -1
        guard completedSize == item.sizeBytes else {
            throw MediaCacheError.sizeMismatch
        }
        guard try sha256Hex(of: partURL) == item.contentHash.lowercased() else {
            throw MediaCacheError.checksumMismatch
        }

        try fileManager.createDirectory(
            at: finalURL.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        if fileManager.fileExists(atPath: finalURL.path) {
            // Another valid sync may have promoted the same immutable content.
            let existingHash = try sha256Hex(of: finalURL)
            guard existingHash == item.contentHash.lowercased() else {
                throw MediaCacheError.checksumMismatch
            }
            try? fileManager.removeItem(at: partURL)
            return
        }
        try fileManager.moveItem(at: partURL, to: finalURL)
    }

    private func appendDownload(
        _ item: RequiredMedia,
        offset: Int64,
        sessionToken: String,
        partURL: URL
    ) async throws {
        let objectURL = apiBaseURL
            .appendingPathComponent("api", isDirectory: true)
            .appendingPathComponent("v1", isDirectory: true)
            .appendingPathComponent("vault", isDirectory: true)
            .appendingPathComponent("objects", isDirectory: true)
            .appendingPathComponent(item.contentHash, isDirectory: false)
        var request = URLRequest(url: objectURL)
        request.setValue("StageCoreSession \(sessionToken)", forHTTPHeaderField: "Authorization")
        if offset > 0 {
            request.setValue("bytes=\(offset)-", forHTTPHeaderField: "Range")
        }

        let (bytes, response) = try await session.bytes(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw MediaCacheError.invalidResponse
        }
        if offset > 0 {
            guard http.statusCode == 206,
                  let range = http.value(forHTTPHeaderField: "Content-Range"),
                  range.hasPrefix("bytes \(offset)-") else {
                throw MediaCacheError.rangeNotHonored
            }
        } else if http.statusCode != 200 {
            throw MediaCacheError.invalidResponse
        }
        if let expectedHash = http.value(forHTTPHeaderField: "X-Content-SHA256"),
           expectedHash.lowercased() != item.contentHash.lowercased() {
            throw MediaCacheError.checksumMismatch
        }

        let handle = try FileHandle(forWritingTo: partURL)
        defer { try? handle.close() }
        try handle.seekToEnd()
        var buffer = Data()
        buffer.reserveCapacity(64 * 1024)
        for try await byte in bytes {
            try Task.checkCancellation()
            buffer.append(byte)
            if buffer.count >= 64 * 1024 {
                try handle.write(contentsOf: buffer)
                buffer.removeAll(keepingCapacity: true)
            }
        }
        if !buffer.isEmpty {
            try handle.write(contentsOf: buffer)
        }
        try handle.synchronize()
    }

    private func sha256Hex(of url: URL) throws -> String {
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        var hasher = SHA256()
        while true {
            let data = try handle.read(upToCount: 1024 * 1024) ?? Data()
            if data.isEmpty { break }
            hasher.update(data: data)
        }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    private static func prepareDirectories(cacheRoot: URL, fileManager: FileManager) throws {
        try fileManager.createDirectory(
            at: cacheRoot.appendingPathComponent("staging", isDirectory: true),
            withIntermediateDirectories: true
        )
        try fileManager.createDirectory(
            at: cacheRoot.appendingPathComponent("objects", isDirectory: true)
                .appendingPathComponent("sha256", isDirectory: true),
            withIntermediateDirectories: true
        )
    }

    private static func validateAPIURL(_ url: URL, policy: CompanionTransportSecurityPolicy) throws {
        guard let scheme = url.scheme?.lowercased() else {
            throw MediaCacheError.secureTransportRequired
        }
        if scheme == "https" { return }
        guard scheme == "http", case .allowInsecureLoopbackForTesting = policy else {
            throw MediaCacheError.secureTransportRequired
        }
        guard let host = url.host?.lowercased(),
              host == "localhost" || host == "127.0.0.1" || host == "::1" else {
            throw MediaCacheError.invalidLoopbackHost
        }
    }
}
#endif

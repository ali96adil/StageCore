#if os(macOS)
import CryptoKit
import Foundation

extension MediaCacheSynchronizer: VisualMediaResolver {
    public func verifiedMediaURL(contentHash: String) async -> URL? {
        guard contentHash.count == 64,
              contentHash == contentHash.lowercased(),
              contentHash.unicodeScalars.allSatisfy({ scalar in
                  (scalar.value >= 48 && scalar.value <= 57) ||
                  (scalar.value >= 97 && scalar.value <= 102)
              }) else {
            return nil
        }

        let url = verifiedObjectURL(for: contentHash)
        guard FileManager.default.fileExists(atPath: url.path),
              let digest = try? visualSHA256Hex(of: url),
              digest == contentHash else {
            return nil
        }
        return url
    }

    private func visualSHA256Hex(of url: URL) throws -> String {
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
}
#endif

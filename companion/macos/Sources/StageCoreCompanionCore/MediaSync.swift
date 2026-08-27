import Foundation

public enum MediaSyncResult: Sendable, Equatable {
    case ready
    case mismatch(String)
    case failed(String)
}

public protocol CompanionMediaSynchronizer: Sendable {
    func synchronize(requiredMedia: [RequiredMedia], sessionToken: String) async -> MediaSyncResult
}

public struct NoopMediaSynchronizer: CompanionMediaSynchronizer {
    public init() {}

    public func synchronize(requiredMedia: [RequiredMedia], sessionToken: String) async -> MediaSyncResult {
        requiredMedia.contains(where: \.required)
            ? .failed("required media synchronizer is not configured")
            : .ready
    }
}

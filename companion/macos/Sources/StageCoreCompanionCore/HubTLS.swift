import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
#if canImport(Security)
import Security
#endif

public enum HubTLS {
    public static func isValidCertificateSHA256(_ value: String) -> Bool {
        let normalized = value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard normalized.count == 64 else { return false }
        return normalized.unicodeScalars.allSatisfy {
            ("0"..."9").contains(Character(String($0))) || ("a"..."f").contains(Character(String($0)))
        }
    }

    public static func makeSession(pinnedCertificateSHA256: String?) -> URLSession {
        #if canImport(Security)
        if let pin = pinnedCertificateSHA256?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(),
           isValidCertificateSHA256(pin) {
            return URLSession(
                configuration: .ephemeral,
                delegate: HubTLSPinningDelegate(expectedCertificateSHA256: pin),
                delegateQueue: nil
            )
        }
        #endif
        return URLSession(configuration: .ephemeral)
    }
}

#if canImport(Security)
private final class HubTLSPinningDelegate: NSObject, URLSessionDelegate, @unchecked Sendable {
    private let expectedCertificateSHA256: String

    init(expectedCertificateSHA256: String) {
        self.expectedCertificateSHA256 = expectedCertificateSHA256
    }

    func urlSession(
        _ session: URLSession,
        didReceive challenge: URLAuthenticationChallenge,
        completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void
    ) {
        guard challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
              let trust = challenge.protectionSpace.serverTrust,
              let certificate = SecTrustGetCertificateAtIndex(trust, 0) else {
            completionHandler(.performDefaultHandling, nil)
            return
        }
        let certificateData = SecCertificateCopyData(certificate) as Data
        let actual = StageCoreSHA256.hexDigest(certificateData)
        guard actual == expectedCertificateSHA256 else {
            completionHandler(.cancelAuthenticationChallenge, nil)
            return
        }
        completionHandler(.useCredential, URLCredential(trust: trust))
    }
}
#endif

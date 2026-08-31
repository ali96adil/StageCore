import Foundation
import Testing
@testable import StageCoreCompanionCore

@Test("SHA-256 matches standard vectors")
func sha256MatchesStandardVectors() {
    #expect(StageCoreSHA256.hexDigest(Data()) == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
    #expect(StageCoreSHA256.hexDigest(Data("abc".utf8)) == "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
}

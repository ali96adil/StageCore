import Foundation
import StageCoreCompanionCore

@main
struct StageCoreCompanionCLI {
    static func main() async {
        let args = CommandLine.arguments
        let hub = argument("--hub", in: args) ?? "ws://127.0.0.1:18083/companion"
        let identityPath = argument("--identity", in: args) ?? "/tmp/stagecore-spk03/companion-id"

        do {
            let store = FileIdentityStore(url: URL(fileURLWithPath: identityPath))
            let id = try store.loadOrCreate()
            guard let url = URL(string: hub) else { throw URLError(.badURL) }
            let client = CompanionClient(url: url, companionID: id)
            try await client.run()
        } catch {
            FileHandle.standardError.write(Data("StageCore Companion spike failed: \(error)\n".utf8))
            exit(1)
        }
    }

    static func argument(_ name: String, in args: [String]) -> String? {
        guard let index = args.firstIndex(of: name), args.indices.contains(index + 1) else { return nil }
        return args[index + 1]
    }
}

import Foundation
import Testing
@testable import CairnOps

struct DevicePairingLinkTests {
    private let token = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

    @Test("Le QR CairnOps fournit une instance normalisée et son secret")
    func parsesPairingURL() throws {
        let url = try #require(URL(
            string: "cairnops://pair?instance=https%3A%2F%2Fcairnops.example.net%2Fops%2F&token=\(token)"
        ))

        let pairing = try DevicePairingLink(url: url)

        #expect(pairing.instanceURL.absoluteString == "https://cairnops.example.net/ops")
        #expect(pairing.token == token)
    }

    @Test(
        "Les liens qui ne prouvent pas un appairage CairnOps sont refusés",
        arguments: [
            "https://pair?instance=https%3A%2F%2Fcairnops.example.net&token=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
            "cairnops://other?instance=https%3A%2F%2Fcairnops.example.net&token=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
            "cairnops://pair?instance=file%3A%2F%2F%2Ftmp%2Fcairnops&token=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
            "cairnops://pair?instance=cairnops.example.net&token=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
            "cairnops://pair?instance=https%3A%2F%2Fcairnops.example.net%3Fredirect%3Devil&token=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
            "cairnops://pair?instance=https%3A%2F%2Fuser%3Apass%40cairnops.example.net&token=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
            "cairnops://pair?instance=https%3A%2F%2Fcairnops.example.net&token=too-short",
            "cairnops://pair?instance=https%3A%2F%2Fcairnops.example.net&token=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&token=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
        ]
    )
    func rejectsInvalidPairingURL(rawValue: String) throws {
        let url = try #require(URL(string: rawValue))

        #expect(throws: DevicePairingLink.ValidationError.self) {
            try DevicePairingLink(url: url)
        }
    }
}

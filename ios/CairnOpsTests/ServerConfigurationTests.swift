import Foundation
import XCTest
@testable import CairnOps

final class ServerConfigurationTests: XCTestCase {
    func testBareHostIsNormalizedToHTTPS() throws {
        let configuration = try ServerConfiguration(
            baseURLString: "cairnops.example.net/"
        ).validated()

        XCTAssertEqual(configuration.baseURLString, "https://cairnops.example.net")
    }

    func testEventURLKeepsBasePathAndCursor() throws {
        let configuration = try ServerConfiguration(
            baseURLString: "https://demo.example.net/cairnops"
        ).validated()

        let eventsURL = try configuration.eventsURL(after: 42)

        XCTAssertEqual(
            eventsURL.absoluteString,
            "https://demo.example.net/cairnops/api/v1/events?after=42"
        )
    }

    func testUppercaseHTTPSIsNormalized() throws {
        let configuration = try ServerConfiguration(
            baseURLString: "HTTPS://demo.example.net/cairnops"
        ).validated()

        XCTAssertEqual(configuration.baseURLString, "https://demo.example.net/cairnops")
        XCTAssertEqual(try configuration.eventsURL(after: nil).scheme, "https")
    }

    func testCredentialsEmbeddedInURLAreRejected() {
        XCTAssertThrowsError(
            try ServerConfiguration(
                baseURLString: "https://operator:secret@demo.example.net"
            ).validated()
        )
    }

    func testLegacyUsernameFieldIsIgnoredWhenLoadingConfiguration() throws {
        let data = Data(
            #"{"baseURLString":"https://demo.example.net","username":"ops"}"#.utf8
        )

        let configuration = try JSONDecoder().decode(ServerConfiguration.self, from: data)

        XCTAssertEqual(configuration.baseURLString, "https://demo.example.net")
    }
}

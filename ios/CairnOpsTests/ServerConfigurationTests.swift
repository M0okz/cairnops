import XCTest
@testable import CairnOps

final class ServerConfigurationTests: XCTestCase {
    func testBareHostIsNormalizedToHTTPS() throws {
        let configuration = try ServerConfiguration(
            baseURLString: "cairnops.example.net/",
            username: "ops"
        ).validated()

        XCTAssertEqual(configuration.baseURLString, "https://cairnops.example.net")
    }

    func testEventURLKeepsBasePathAndCursor() throws {
        let configuration = try ServerConfiguration(
            baseURLString: "https://demo.example.net/cairnops",
            username: "ops"
        ).validated()

        let eventsURL = try configuration.eventsURL(after: 42)

        XCTAssertEqual(
            eventsURL.absoluteString,
            "https://demo.example.net/cairnops/api/v1/events?after=42"
        )
    }
}

// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "TraceGenerator",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .executable(
            name: "trace-generator",
            targets: ["TraceGenerator"]
        )
    ],
    targets: [
        .executableTarget(
            name: "TraceGenerator",
            path: "Sources"
        )
    ]
)

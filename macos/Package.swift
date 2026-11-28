// swift-tools-version: 6.1
import PackageDescription

let package = Package(
    name: "SoloDawnPermissions",
    platforms: [
        .macOS(.v13),
    ],
    dependencies: [
        .package(url: "https://github.com/jaywcjlove/PermissionFlow.git", exact: "2.11.4"),
    ],
    targets: [
        .executableTarget(
            name: "SoloDawnPermissions",
            dependencies: [
                .product(name: "PermissionFlow", package: "PermissionFlow"),
            ]
        ),
        .testTarget(
            name: "SoloDawnPermissionsTests",
            dependencies: [
                "SoloDawnPermissions",
            ]
        ),
    ]
)

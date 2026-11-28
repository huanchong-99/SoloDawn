import Foundation
import PermissionFlow
import Testing
@testable import SoloDawnPermissions

private final class TestBundleAnchor {}

private let registeredSiblingResourceBundle: Void = {
    // Xcode's `swift test` runs `<build>/SoloDawnPermissionsPackageTests.xctest` inside Xcode's
    // external test runner (`Bundle.main`) and emits `PermissionFlow_PermissionFlow.bundle` beside
    // `.xctest`. `PermissionFlowResources` checks inside `.xctest` and scans `Bundle.allBundles`,
    // but omits `.xctest`'s parent directory; instantiating the sibling bundle registers it in
    // `Bundle.allBundles` before `PermissionFlowResources.packageBundle` caches its lookup.
    let siblingURL = Bundle(for: TestBundleAnchor.self).bundleURL
        .deletingLastPathComponent()
        .appendingPathComponent("\(PermissionFlowResources.resourceBundleName).bundle")
    _ = Bundle(url: siblingURL)
}()

struct SoloDawnPermissionsTests {
    init() {
        _ = registeredSiblingResourceBundle
    }

    @Test func langMapping() {
        #expect(HelperArguments.localeIdentifier(arguments: ["SoloDawnPermissions"]) == "en")
        #expect(HelperArguments.localeIdentifier(arguments: ["SoloDawnPermissions", "--lang", "en"]) == "en")
        #expect(HelperArguments.localeIdentifier(arguments: ["SoloDawnPermissions", "--lang", "zh-CN"]) == "zh-Hans")
        #expect(HelperArguments.localeIdentifier(arguments: ["SoloDawnPermissions", "--lang=zh-CN"]) == "zh-Hans")
        #expect(HelperArguments.localeIdentifier(arguments: ["SoloDawnPermissions", "--lang", "zh-TW"]) == "en")
        #expect(HelperArguments.localeIdentifier(arguments: ["SoloDawnPermissions", "--lang", "fr"]) == "en")
        #expect(HelperArguments.localeIdentifier(arguments: ["SoloDawnPermissions", "--lang"]) == "en")
        #expect(HelperArguments.localeIdentifier(arguments: ["SoloDawnPermissions", "--lang", "zh-CN", "--lang", "en"]) == "en")
    }

    @Test func fullDiskAccessSettingsURL() {
        #expect(
            PermissionFlowPane.fullDiskAccess.settingsURL.absoluteString
                == "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_AllFiles"
        )
        #expect(PermissionFlowPane.fullDiskAccess.supportsFloatingAuthorizationPanel)
    }

    @Test func zhHansResourceBundleLocalization() throws {
        let bundle = try #require(PermissionFlowResources.packageBundle)
        #expect(bundle.bundleURL.lastPathComponent == "PermissionFlow_PermissionFlow.bundle")
        let zhLocalization = try #require(
            Bundle.preferredLocalizations(from: bundle.localizations, forPreferences: ["zh-Hans"]).first
        )
        #expect(zhLocalization.lowercased() == "zh-hans")
        #expect(bundle.path(forResource: zhLocalization, ofType: "lproj") != nil)

        let paneTitle = PermissionFlowResources.localizedString(
            for: "permission_flow.pane.full_disk_access",
            localeIdentifier: "zh-Hans"
        )
        let dragLabel = PermissionFlowResources.localizedString(
            for: "permission_flow.drag.label",
            localeIdentifier: "zh-Hans"
        )
        let englishTitle = PermissionFlowResources.localizedString(
            for: "permission_flow.pane.full_disk_access",
            localeIdentifier: "en"
        )

        #expect(paneTitle == "完全磁盘访问")
        #expect(dragLabel == "拖拽")
        #expect(englishTitle == "Full Disk Access")
    }

    @Test func verifyPackagedResourcesChecksAppBundleAndStrings() throws {
        #expect(!HelperArguments.shouldVerifyResources(arguments: ["SoloDawnPermissions", "--lang", "en"]))
        #expect(HelperArguments.shouldVerifyResources(arguments: ["SoloDawnPermissions", "--verify-resources"]))

        let sourceBundle = try #require(PermissionFlowResources.packageBundle)
        let tempRoot = FileManager.default.temporaryDirectory
            .appendingPathComponent("SoloDawnPermTest-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: tempRoot) }

        let appURL = tempRoot.appendingPathComponent("SoloDawn.app", isDirectory: true)
        let contentsURL = appURL.appendingPathComponent("Contents", isDirectory: true)
        let resourcesURL = contentsURL.appendingPathComponent("Resources", isDirectory: true)
        try FileManager.default.createDirectory(at: resourcesURL, withIntermediateDirectories: true)

        let plist = """
        <?xml version="1.0" encoding="UTF-8"?>
        <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
        <plist version="1.0"><dict>
          <key>CFBundleIdentifier</key><string>org.solodawn.test</string>
          <key>CFBundlePackageType</key><string>APPL</string>
        </dict></plist>
        """
        try Data(plist.utf8).write(to: contentsURL.appendingPathComponent("Info.plist"))
        let copiedURL = resourcesURL.appendingPathComponent("\(PermissionFlowResources.resourceBundleName).bundle")
        try FileManager.default.copyItem(at: sourceBundle.bundleURL, to: copiedURL)

        let appBundle = try #require(Bundle(url: appURL))
        let copiedBundle = try #require(Bundle(url: copiedURL))
        #expect(HelperArguments.verifyPackagedResources(mainBundle: appBundle, packageBundle: copiedBundle) == nil)
        #expect(HelperArguments.verifyPackagedResources(mainBundle: appBundle, packageBundle: nil) != nil)
        #expect(HelperArguments.verifyPackagedResources(mainBundle: appBundle, packageBundle: sourceBundle) != nil)
        #expect(HelperArguments.verifyPackagedResources(mainBundle: Bundle(for: TestBundleAnchor.self), packageBundle: copiedBundle) != nil)
    }
}

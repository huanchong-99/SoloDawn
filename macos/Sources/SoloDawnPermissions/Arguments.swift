import Foundation
import PermissionFlow

enum HelperArguments {
    static func localeIdentifier(arguments: [String]) -> String {
        var lang = ""
        var index = arguments.startIndex
        while index < arguments.endIndex {
            let arg = arguments[index]
            if arg == "--lang" {
                let next = arguments.index(after: index)
                if next < arguments.endIndex {
                    lang = arguments[next]
                    index = arguments.index(after: next)
                    continue
                }
            } else if arg.hasPrefix("--lang=") {
                lang = String(arg.dropFirst("--lang=".count))
            }
            index = arguments.index(after: index)
        }
        return lang == "zh-CN" ? "zh-Hans" : "en"
    }

    static func shouldVerifyResources(arguments: [String]) -> Bool {
        arguments.dropFirst().contains("--verify-resources")
    }

    static func verifyPackagedResources(
        mainBundle: Bundle = .main,
        packageBundle: Bundle? = PermissionFlowResources.packageBundle
    ) -> String? {
        guard mainBundle.bundleURL.pathExtension == "app",
              let resourceURL = mainBundle.resourceURL else {
            return "expected .app main bundle, got \(mainBundle.bundleURL.path)"
        }
        let expectedURL = resourceURL
            .appendingPathComponent("\(PermissionFlowResources.resourceBundleName).bundle")
            .standardizedFileURL
        guard let packageBundle else {
            return "PermissionFlowResources.packageBundle is nil"
        }
        guard packageBundle.bundleURL.standardizedFileURL == expectedURL else {
            return "expected resource bundle at \(expectedURL.path), got \(packageBundle.bundleURL.path)"
        }
        let zhPane = PermissionFlowResources.localizedString(
            for: "permission_flow.pane.full_disk_access",
            localeIdentifier: "zh-Hans"
        )
        let zhDrag = PermissionFlowResources.localizedString(
            for: "permission_flow.drag.label",
            localeIdentifier: "zh-Hans"
        )
        let enPane = PermissionFlowResources.localizedString(
            for: "permission_flow.pane.full_disk_access",
            localeIdentifier: "en"
        )
        guard zhPane == "完全磁盘访问", zhDrag == "拖拽", enPane == "Full Disk Access" else {
            return "unexpected localized strings: zhPane=\(zhPane), zhDrag=\(zhDrag), enPane=\(enPane)"
        }
        return nil
    }
}

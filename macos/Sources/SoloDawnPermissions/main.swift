import AppKit
import Foundation
import PermissionFlow

if HelperArguments.shouldVerifyResources(arguments: CommandLine.arguments) {
    if let problem = HelperArguments.verifyPackagedResources() {
        FileHandle.standardError.write(Data((problem + "\n").utf8))
        Foundation.exit(1)
    }
    Foundation.exit(0)
}

@MainActor
final class HelperAppDelegate: NSObject, NSApplicationDelegate {
    private let locale: String
    private var controller: PermissionFlowController?

    init(locale: String) {
        self.locale = locale
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        let controller = PermissionFlow.makeController(
            configuration: .init(
                requiredAppURLs: [Bundle.main.bundleURL],
                localeIdentifier: locale
            )
        )
        self.controller = controller

        // Defer exit to the next main-queue turn so closePanel(returnToPreviousApp: true)
        // finishes reactivatePreviousFrontmostApplication() after panel.close() posts willClose.
        _ = NotificationCenter.default.addObserver(
            forName: NSWindow.willCloseNotification,
            object: nil,
            queue: .main
        ) { note in
            guard note.object is NSPanel else { return }
            DispatchQueue.main.async {
                Foundation.exit(0)
            }
        }

        let pointer = NSEvent.mouseLocation
        controller.authorize(
            pane: .fullDiskAccess,
            sourceFrameInScreen: CGRect(x: pointer.x, y: pointer.y, width: 1, height: 1)
        )

        Thread.detachNewThread { [weak self] in
            let gotLine = readLine(strippingNewline: false) != nil
            DispatchQueue.main.async {
                MainActor.assumeIsolated {
                    if gotLine {
                        self?.controller?.closePanel(returnToPreviousApp: true)
                    }
                }
                DispatchQueue.main.async {
                    Foundation.exit(0)
                }
            }
        }
    }
}

let locale = HelperArguments.localeIdentifier(arguments: CommandLine.arguments)
let app = NSApplication.shared
let delegate = HelperAppDelegate(locale: locale)
app.delegate = delegate
app.setActivationPolicy(.accessory)
app.run()

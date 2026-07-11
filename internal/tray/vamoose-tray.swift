// vamoose-tray is the macOS menu bar companion: a moose in the status bar that shows
// the holds the daemon is watching, lets you check, promote, or cancel one, and opens
// the dashboard. It talks to the local vamoose app server over loopback, and starts
// that server, plus the daemon, when they are not already running, so clicking the
// moose is all the ambient lifecycle a user needs. Build with `make tray`.

import AppKit
import Foundation

// baseURL is the local dashboard server the tray reads from and acts through.
let baseURL = URL(string: "http://127.0.0.1:8787")!

// Watch mirrors one entry of /api/watches.
struct Watch: Decodable {
    let provider: String
    let holdID: String
    let workflow: String
    let step: Int
    let approver: String?
    let subject: String?

    enum CodingKeys: String, CodingKey {
        case provider
        case holdID = "hold_id"
        case workflow, step, approver, subject
    }
}

// Event mirrors one entry of /api/history.
struct Event: Decodable {
    let time: String?
    let workflow: String?
    let action: String
    let actor: String?
    let holdID: String?

    enum CodingKeys: String, CodingKey {
        case time, workflow, action, actor
        case holdID = "hold_id"
    }
}

// holdAction carries a menu item's action target through representedObject.
final class HoldAction: NSObject {
    let action: String
    let holdID: String
    let provider: String
    init(_ action: String, _ holdID: String, _ provider: String) {
        self.action = action
        self.holdID = holdID
        self.provider = provider
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate, NSMenuDelegate {
    var statusItem: NSStatusItem!
    var serverProcess: Process?
    var daemonProcess: Process?
    var watches: [Watch] = []
    var history: [Event] = []
    var version = ""
    var serverUp = false

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem.button?.title = "🫎"
        let menu = NSMenu()
        menu.delegate = self
        statusItem.menu = menu

        ensureServices()
        refresh()
        Timer.scheduledTimer(withTimeInterval: 60, repeats: true) { _ in self.refresh() }
    }

    func applicationWillTerminate(_ notification: Notification) {
        serverProcess?.terminate()
        daemonProcess?.terminate()
    }

    // vamooseBinary locates the vamoose CLI: the copy bundled next to this executable,
    // an explicit override, the Homebrew paths, then a PATH lookup.
    func vamooseBinary() -> String? {
        var candidates: [String] = []
        if let override = ProcessInfo.processInfo.environment["VAMOOSE_TRAY_BIN"] {
            candidates.append(override)
        }
        let sibling = (Bundle.main.executablePath as NSString?)?
            .deletingLastPathComponent.appending("/vamoose")
        if let sibling { candidates.append(sibling) }
        candidates.append("/opt/homebrew/bin/vamoose")
        candidates.append("/usr/local/bin/vamoose")
        for c in candidates where FileManager.default.isExecutableFile(atPath: c) {
            return c
        }
        return nil
    }

    // ensureServices starts the dashboard server and the daemon when the server is not
    // already answering, so the tray works from a cold start but attaches to an
    // existing `vamoose app` rather than fighting it for the port.
    func ensureServices() {
        healthCheck { up in
            self.serverUp = up
            if up { return }
            // Do not stack children: if a spawn is already alive, give it time to bind.
            if self.serverProcess?.isRunning == true { return }
            guard let bin = self.vamooseBinary() else { return }
            self.serverProcess = self.spawn(bin, ["app", "--no-open"])
            if self.daemonProcess?.isRunning != true {
                self.daemonProcess = self.spawn(bin, ["daemon"])
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) { self.refresh() }
        }
    }

    // spawn runs the vamoose binary with args as a child this app owns.
    func spawn(_ bin: String, _ args: [String]) -> Process? {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: bin)
        p.arguments = args
        p.standardOutput = FileHandle.nullDevice
        p.standardError = FileHandle.nullDevice
        do {
            try p.run()
            return p
        } catch {
            return nil
        }
    }

    // healthCheck reports whether the dashboard server answers on loopback.
    func healthCheck(_ done: @escaping (Bool) -> Void) {
        var req = URLRequest(url: baseURL.appendingPathComponent("health"))
        req.timeoutInterval = 1
        URLSession.shared.dataTask(with: req) { _, resp, _ in
            let ok = (resp as? HTTPURLResponse)?.statusCode == 200
            DispatchQueue.main.async { done(ok) }
        }.resume()
    }

    // fetch decodes a JSON API response on the main queue, delivering nil on any error.
    func fetch<T: Decodable>(_ path: String, _ type: T.Type, _ done: @escaping (T?) -> Void) {
        var req = URLRequest(url: baseURL.appendingPathComponent(path))
        req.timeoutInterval = 3
        URLSession.shared.dataTask(with: req) { data, _, _ in
            let out = data.flatMap { try? JSONDecoder().decode(type, from: $0) }
            DispatchQueue.main.async { done(out) }
        }.resume()
    }

    // refresh reloads the watch list, history, and version, and updates the badge.
    func refresh() {
        healthCheck { up in
            self.serverUp = up
            if !up {
                self.statusItem.button?.title = "🫎"
                return
            }
            self.fetch("api/watches", [Watch]?.self) { w in
                self.watches = (w ?? nil) ?? []
                let n = self.watches.count
                self.statusItem.button?.title = n > 0 ? "🫎 \(n)" : "🫎"
            }
            self.fetch("api/history", [Event]?.self) { h in
                self.history = Array((((h ?? nil) ?? []).suffix(5)).reversed())
            }
            var req = URLRequest(url: baseURL.appendingPathComponent("api/version"))
            req.timeoutInterval = 2
            URLSession.shared.dataTask(with: req) { data, _, _ in
                let v = data.flatMap { String(data: $0, encoding: .utf8) } ?? ""
                DispatchQueue.main.async { self.version = v }
            }.resume()
        }
    }

    // menuNeedsUpdate rebuilds the dropdown from the latest state each time it opens.
    func menuNeedsUpdate(_ menu: NSMenu) {
        menu.removeAllItems()

        if !serverUp {
            menu.addItem(disabled("Starting the vamoose server…"))
            ensureServices()
        } else if watches.isEmpty {
            menu.addItem(disabled("Nothing is being watched"))
        } else {
            menu.addItem(disabled("Watching"))
            for w in watches {
                let title = "\(w.subject ?? w.holdID)  (\(w.workflow)\(w.approver.map { " · awaiting \($0)" } ?? ""))"
                let item = NSMenuItem(title: title, action: nil, keyEquivalent: "")
                let sub = NSMenu()
                for act in ["check", "promote", "cancel"] {
                    let a = NSMenuItem(title: act.capitalized, action: #selector(runHoldAction(_:)), keyEquivalent: "")
                    a.target = self
                    a.representedObject = HoldAction(act, w.holdID, w.provider)
                    sub.addItem(a)
                }
                item.submenu = sub
                menu.addItem(item)
            }
        }

        if !history.isEmpty {
            menu.addItem(.separator())
            menu.addItem(disabled("Recent"))
            for e in history {
                let when = (e.time ?? "").replacingOccurrences(of: "T", with: " ").prefix(16)
                let who = e.actor.map { " · \($0)" } ?? ""
                menu.addItem(disabled("\(e.action)  \(e.workflow ?? "")\(who)  \(when)"))
            }
        }

        menu.addItem(.separator())
        let open = NSMenuItem(title: "Open dashboard", action: #selector(openDashboard), keyEquivalent: "d")
        open.target = self
        menu.addItem(open)
        let refreshItem = NSMenuItem(title: "Refresh", action: #selector(refreshNow), keyEquivalent: "r")
        refreshItem.target = self
        menu.addItem(refreshItem)
        menu.addItem(.separator())
        if !version.isEmpty {
            menu.addItem(disabled(version))
        }
        let quit = NSMenuItem(title: "Quit Vamoose Tray", action: #selector(quit), keyEquivalent: "q")
        quit.target = self
        menu.addItem(quit)
    }

    // disabled returns a non-clickable informational menu line.
    func disabled(_ title: String) -> NSMenuItem {
        let item = NSMenuItem(title: title, action: nil, keyEquivalent: "")
        item.isEnabled = false
        return item
    }

    // runHoldAction posts a check, promote, or cancel for a watched hold.
    @objc func runHoldAction(_ sender: NSMenuItem) {
        guard let ha = sender.representedObject as? HoldAction else { return }
        var req = URLRequest(url: baseURL.appendingPathComponent("api/action"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let body: [String: String] = ["action": ha.action, "holdID": ha.holdID, "provider": ha.provider]
        req.httpBody = try? JSONSerialization.data(withJSONObject: body)
        URLSession.shared.dataTask(with: req) { _, _, _ in
            DispatchQueue.main.async { self.refresh() }
        }.resume()
    }

    @objc func openDashboard() {
        NSWorkspace.shared.open(baseURL)
    }

    @objc func refreshNow() {
        refresh()
    }

    @objc func quit() {
        NSApp.terminate(nil)
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()

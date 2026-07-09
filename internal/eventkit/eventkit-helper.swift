// vamoose-eventkit reads attendee responses from the local macOS Calendar.app via
// EventKit. iCloud does not report attendee accept/decline over CalDAV, but the
// Apple-synced local copy does, so this recovers approval detection on a Mac.
//
// Build:  make eventkit   (embeds Info.plist so macOS shows the access prompt)
// Usage:  vamoose-eventkit status <ical-uid>   ->  JSON on stdout
//
// The first run prompts for calendar access. Grant it once. The embedded
// Info.plist carries NSCalendarsFullAccessUsageDescription; without it macOS 14+
// denies calendar access without prompting.
import EventKit
import Foundation

// statusString maps an EventKit participant status to a vamoose response word.
func statusString(_ s: EKParticipantStatus) -> String {
    switch s {
    case .accepted:  return "accepted"
    case .declined:  return "declined"
    case .tentative: return "tentative"
    case .pending:   return "pending"
    default:         return "unknown"
    }
}

let args = CommandLine.arguments
guard args.count >= 3, args[1] == "status" else {
    FileHandle.standardError.write(Data("usage: vamoose-eventkit status <uid>\n".utf8))
    exit(2)
}
let uid = args[2]

let store = EKEventStore()
let sem = DispatchSemaphore(value: 0)
var granted = false
if #available(macOS 14.0, *) {
    store.requestFullAccessToEvents { ok, _ in granted = ok; sem.signal() }
} else {
    store.requestAccess(to: .event) { ok, _ in granted = ok; sem.signal() }
}
sem.wait()
guard granted else {
    FileHandle.standardError.write(Data("calendar access not granted\n".utf8))
    exit(1)
}

// calendarItems(withExternalIdentifier:) matches the iCalendar UID vamoose set.
var attendeesOut: [[String: String]] = []
for item in store.calendarItems(withExternalIdentifier: uid) {
    guard let ev = item as? EKEvent, let attendees = ev.attendees else { continue }
    for a in attendees {
        var email = ""
        if a.url.scheme == "mailto" {
            email = a.url.absoluteString.replacingOccurrences(of: "mailto:", with: "")
        }
        attendeesOut.append(["email": email, "status": statusString(a.participantStatus)])
    }
}

let payload: [String: Any] = ["uid": uid, "attendees": attendeesOut]
let data = try JSONSerialization.data(withJSONObject: payload)
FileHandle.standardOutput.write(data)
FileHandle.standardOutput.write(Data("\n".utf8))

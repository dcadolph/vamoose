# Regenerate the demo GIF from the repo root:
#   go build -o vamoose . && PATH="$PWD:$PATH" vhs assets/demo.tape
# Requires vhs (github.com/charmbracelet/vhs). The dry-run flow needs no account.

Output assets/vamoose-demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1340
Set Height 720
Set Padding 32
Set Theme "Builtin Dark"
Set TypingSpeed 45ms

Hide
Type "export VAMOOSE_CLIENT_ID=demo PATH=$PWD:$PATH PS1='$ '" Enter
Type "export VAMOOSE_AUDIT_FILE=/tmp/vamoose-demo-audit.json" Enter
Type `printf '[{"time":"2026-07-11T09:00:00Z","workflow":"pto","provider":"google","hold_id":"EVT-8841","action":"created"},{"time":"2026-07-11T10:12:00Z","workflow":"pto","provider":"google","hold_id":"EVT-8841","action":"approved","actor":"boss@work.com"},{"time":"2026-07-11T10:12:01Z","workflow":"pto","provider":"google","hold_id":"EVT-8841","action":"notified"},{"time":"2026-07-11T10:12:02Z","workflow":"pto-file-leave","provider":"google","hold_id":"EVT-8841","action":"filed"}]' > /tmp/vamoose-demo-audit.json` Enter
Type "clear" Enter
Show

Type "vamoose workflows"
Sleep 500ms
Enter
Sleep 2500ms

Type "vamoose off next week --manager boss@example.com --dry-run"
Sleep 500ms
Enter
Sleep 3s

Type "vamoose run pto-file-leave next week --manager boss@example.com --dry-run"
Sleep 500ms
Enter
Sleep 3s

Type "vamoose history"
Sleep 500ms
Enter
Sleep 3500ms

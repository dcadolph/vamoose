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

Type "vamoose run pto next week --manager boss@example.com --dry-run"
Sleep 500ms
Enter
Sleep 3s

Type "vamoose run away --start 2026-08-03 --end 2026-08-07 --dry-run"
Sleep 500ms
Enter
Sleep 3500ms

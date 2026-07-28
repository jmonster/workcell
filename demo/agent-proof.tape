Output workcell-real-agent-proof-frames/

Require codex
Require go

Set Shell "bash"
Set Width 1920
Set Height 1200
Set FontFamily "Menlo"
Set FontSize 24
Set LineHeight 1.1
Set Framerate 30
Set TypingSpeed 16ms
Set Padding 0
Set Theme { "name": "Workcell Proof", "black": "#171717", "red": "#ff5f5f", "green": "#5fff87", "yellow": "#ffd75f", "blue": "#5f87ff", "magenta": "#d787ff", "cyan": "#5fd7ff", "white": "#f2f2e8", "brightBlack": "#767676", "brightRed": "#ff5f87", "brightGreen": "#87ffaf", "brightYellow": "#ffff87", "brightBlue": "#87afff", "brightMagenta": "#ff87ff", "brightCyan": "#87ffff", "brightWhite": "#ffffff", "background": "#171717", "foreground": "#f2f2e8", "selection": "#3a3a3a", "cursor": "#f2f2e8" }

Hide
Type 'cd "$WORKCELL_REPO"'
Enter
Type "rm -rf /tmp/workcell-agent-proof-site-recording"
Enter
Type "clear"
Enter
Show

Type "./demo/agent-proof/run.sh --output /tmp/workcell-agent-proof-site-recording"
Enter

Wait+Screen@150s /CONTROL COMPLETE/
Sleep 4s
Right

Wait+Screen@240s /WITH WORKCELL COMPLETE/
Sleep 6s
Right

Wait+Screen@10s /FINAL SCORECARD/
Sleep 4s
Right
Sleep 1s

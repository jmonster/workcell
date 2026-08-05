Output ../site/src/assets/demo/stringproof-cli-proof.gif

Require python3

Set Shell "bash"
Set Width 1600
Set Height 560
Set FontFamily "Menlo"
Set FontSize 18
Set LineHeight 1.2
Set Framerate 24
Set TypingSpeed 14ms
Set Padding 32
Set Theme { "name": "Stringproof Proof", "black": "#171717", "red": "#ff5f5f", "green": "#5fff87", "yellow": "#ffd75f", "blue": "#5f87ff", "magenta": "#d787ff", "cyan": "#5fd7ff", "white": "#f2f2e8", "brightBlack": "#767676", "brightRed": "#ff5f87", "brightGreen": "#87ffaf", "brightYellow": "#ffff87", "brightBlue": "#87afff", "brightMagenta": "#ff87ff", "brightCyan": "#87ffff", "brightWhite": "#ffffff", "background": "#171717", "foreground": "#f2f2e8", "selection": "#3a3a3a", "cursor": "#f2f2e8" }

Hide
Type "rm -rf /tmp/stringproof-proof && mkdir -p /tmp/stringproof-proof"
Enter
Type 'cp "$STRINGPROOF_ZIPAPP" /tmp/stringproof-proof/stringproof && chmod +x /tmp/stringproof-proof/stringproof'
Enter
Type "export PATH=/tmp/stringproof-proof:$PATH"
Enter
Type 'cd "$STRINGPROOF_REPO"'
Enter
Type "clear"
Enter
Show

Type "stringproof check fixtures/corrupted --source-locale en"
Enter
Sleep 4s
Type 'echo "exit $?"'
Enter
Sleep 2s

Type "stringproof check fixtures/clean --source-locale en"
Enter
Sleep 2s
Type 'echo "exit $?"'
Enter
Sleep 4s

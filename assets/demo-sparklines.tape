Output assets/thermal-sparklines.gif

Set Shell "bash"
Set FontSize 13
Set Width 1200
Set Height 350
Set Padding 0

# Start with thermal running, let sparklines fill up
Type "cd /Users/toddwshaffer/Desktop/apps/coolant && ./bin/thermo"
Enter
Sleep 20s

# Quit thermal, spike CPU, relaunch to watch the spike
Type "q"
Sleep 300ms
Type "for i in 1 2 3 4 5 6; do yes > /dev/null & done && ./bin/thermo"
Enter
Sleep 8s

# Quit, kill spike, relaunch to watch decay
Type "q"
Sleep 300ms
Type "pkill -f yes 2>/dev/null; ./bin/thermo"
Enter
Sleep 12s

Type "q"

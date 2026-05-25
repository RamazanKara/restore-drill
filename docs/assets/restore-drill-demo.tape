Output docs/assets/restore-drill-demo.gif

Require docker
Require make

Set Shell "bash"
Set FontFamily "DejaVu Sans Mono"
Set Theme "Catppuccin Mocha"
Set Width 1500
Set Height 900
Set FontSize 22
Set LineHeight 1.2
Set TypingSpeed 24ms
Set Framerate 12
Set PlaybackSpeed 1.6
Set Padding 18
Set WindowBar Colorful

Type "make -s build" Enter
Sleep 4s

Type "./bin/restore-drill validate -c examples/demo-redis-aof.yaml" Enter
Sleep 3s

Type "./bin/restore-drill run -c examples/demo-redis-aof.yaml --runtime docker 2>/tmp/rd.log" Enter
Sleep 20s

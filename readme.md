# Countdown App

This is a simple countdown app built with Go and Ebiten. The application allows you to set a countdown timer, which will be displayed in a floating window. Once the timer completes, the app will terminate. It's a lightweight utility that can be installed and run locally.

Original repo: https://github.com/CatalinPlesu/countdown

**Note**: 95% of this code was written with the assistance of ChatGPT.

## Features

![Example](screenshot.png)

- Set a countdown timer in minutes, or in hours/minutes/seconds.
- Displays the remaining time in hours, minutes, and seconds.
- Blinks yellow/red once under 30 seconds remain, for a clear urgency cue.
- A transparent window with minimal UI, ideal for overlay use.
- The application is windowless (no borders, no title bar).
- Place the window in a specific corner and/or on a specific monitor.
- Remembers its last position and monitor between runs.
- Control what happens once it hits zero: close immediately, keep
  blinking for a set number of extra seconds, or keep blinking until
  interrupted.
- Uses an embedded font, so it doesn't depend on any particular
  distro having a specific font installed at a specific path.
- Resizable via `-scale`, which scales the font and box together.
- Reads defaults from a commented YAML config file, so you don't have
  to repeat your preferred flags every time.

### Install and Compile with Make

To build and install the app, simply run:

```bash
make
```

Install the app already build and present here:
```bash
make install
```

### Windows

The app cross-compiles to Windows with no extra setup (ebiten's desktop
backend is pure Go, no cgo/C toolchain needed):

```bash
make build-windows    # produces bin/countdown.exe
```

Copy `bin/countdown.exe` to the Windows machine and run it from a
terminal (PowerShell or cmd) the same way as on Linux, e.g.:

```powershell
countdown.exe 10
countdown.exe -corner=top-left -scale=1.5 10
```

This is cross-compiled from Linux and hasn't been run/tested on an
actual Windows machine, so treat window transparency/decoration/mouse
passthrough behavior there as unverified.

## Usage

Run `countdown --help` (or `-help`) at any time to see the full flag
reference. Note `-h` is the shorthand for `-hours`, not help.

put the desired minutes for the countdown as the last argument

```bash
countdown minutes
countdown 10
countdown 160
```

Or specify hours/minutes/seconds explicitly with `-hours`/`-minutes`/`-seconds`
(short forms `-h`/`-m`/`-s`), combinable, and instead of the plain-minutes
argument above:

```bash
countdown -h 1 -m 30       # 1 hour 30 minutes
countdown -m 5             # 5 minutes
countdown -s 90            # 90 seconds
countdown --seconds=45     # double-dash works too
```

The window remembers its last position and monitor between runs, saved to:

- Linux: `~/.config/countdown/state.json`
- Windows: `%AppData%\countdown\state.json`
  (typically `C:\Users\<you>\AppData\Roaming\countdown\state.json`)

On first run it defaults to the bottom-right corner of the current monitor.

```bash
countdown -list-monitors          # list available monitors and their index
countdown -corner=top-left 10     # place in a specific corner
countdown -monitor=1 10           # place on a specific monitor
countdown -corner=top-left -monitor=1 10
```

Valid corners: `top-left`, `top-right`, `bottom-left`, `bottom-right`.

By default the window closes the moment the countdown hits zero. Use
`-timeout` to change that: it keeps blinking yellow/red past zero.

```bash
countdown -timeout=0 10    # close immediately at zero (default)
countdown -timeout=30 10   # keep blinking for 30 more seconds, then close
countdown -timeout=-1 10   # keep blinking until interrupted (Ctrl+C)
```

Whenever `-timeout` is non-zero, once the countdown hits zero it starts
counting back up instead, shown as `-HH:MM:SS` (e.g. `-00:00:12`), so
you can see how long it's been since time ran out.

Use `-scale` to resize the whole thing (font and box together):

```bash
countdown -scale=2 10     # twice the size (default is 1)
countdown -scale=0.5 10   # half the size
```

`-padding` sets the base padding in pixels around the text (default 30),
before `-scale` multiplies it:

```bash
countdown -padding=60 10
```

### Config file

Rather than repeating flags every time, you can set defaults in a YAML
config file at the default location below (or point elsewhere with
`-config`). See [`config.example.yaml`](config.example.yaml) for every
available option, commented out with its meaning. Any flag passed on the
command line always overrides the config file; the config file overrides
the app's built-in defaults; a missing config file is fine and just means
built-in defaults apply.

Default config file location:

- Linux: `~/.config/countdown/config.yaml`
- Windows: `%AppData%\countdown\config.yaml`
  (typically `C:\Users\<you>\AppData\Roaming\countdown\config.yaml`)

```bash
# Linux
mkdir -p ~/.config/countdown
cp config.example.yaml ~/.config/countdown/config.yaml
```

```powershell
# Windows (PowerShell)
mkdir "$env:AppData\countdown"
copy config.example.yaml "$env:AppData\countdown\config.yaml"
```

```bash
# edit it to your liking, then just run:
countdown 10

countdown -config=/path/to/other.yaml 10   # or use a different file
```

Or generate/update the config file from your current flags with
`-save-config`, instead of hand-editing it. It merges onto whatever's
already in the file, so passing just the flags you want to change is
enough:

```bash
countdown -scale=1.5 -corner=top-left -timeout=-1 -save-config
```

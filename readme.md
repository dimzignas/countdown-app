# Countdown App

This is a simple countdown app built with Go and Ebiten. The application allows you to set a countdown timer, which will be displayed in a floating window. Once the timer completes, the app will terminate. It's a lightweight utility that can be installed and run locally.

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

### Install and Compile with Make

To build and install the app, simply run:

```bash
make
```

Install the app already build and present here:
```bash
make install
```

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

The window remembers its last position and monitor between runs
(saved to `~/.config/countdown/state.json`). On first run it defaults
to the bottom-right corner of the current monitor.

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

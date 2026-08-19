# Countdown App

This is a simple countdown app built with Go and Ebiten. The application allows you to set a countdown timer, which will be displayed in a floating window. Once the timer completes, the app will terminate. It's a lightweight utility that can be installed and run locally.

**Note**: 95% of this code was written with the assistance of ChatGPT.

## Features

![Example](screenshot.png)

- Set a countdown timer in minutes.
- Displays the remaining time in hours, minutes, and seconds.
- A transparent window with minimal UI, ideal for overlay use.
- The application is windowless (no borders, no title bar).
- Once the countdown is complete, the app will automatically exit.

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

put the desired minutes for the countdown as the last argument

```bash
countdown minutes
countdown 10
countdown 160
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

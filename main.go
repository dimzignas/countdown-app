package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"gopkg.in/yaml.v3"
)

// windowMargin is the gap kept between the window and the screen edge it's
// anchored to, so the countdown isn't flush against the corner.
const windowMargin = 20

// windowState is persisted across runs so the overlay reopens where it was left.
type windowState struct {
	X          int `json:"x"`
	Y          int `json:"y"`
	MonitorIdx int `json:"monitorIdx"`
}

var validCorners = []string{"top-left", "top-right", "bottom-left", "bottom-right"}

// cornerPosition returns the top-left coordinates (relative to the target
// monitor's origin) for placing a window of size windowWidth x windowHeight
// in the given corner of a monitor of size screenWidth x screenHeight, with
// windowMargin of breathing room from the edges.
func cornerPosition(corner string, screenWidth, screenHeight, windowWidth, windowHeight int) (int, int) {
	x, y := windowMargin, windowMargin
	if strings.HasSuffix(corner, "right") {
		x = screenWidth - windowWidth - windowMargin
	}
	if strings.HasPrefix(corner, "bottom") {
		y = screenHeight - windowHeight - windowMargin
	}
	return x, y
}

func statePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "countdown", "state.json"), nil
}

func loadWindowState() (*windowState, bool) {
	path, err := statePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var state windowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, false
	}
	return &state, true
}

func saveWindowState(x, y, monitorIdx int) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(windowState{X: x, Y: y, MonitorIdx: monitorIdx})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// config holds user preferences loaded from a YAML config file. Fields are
// pointers so we can tell "not set in the file" apart from a legitimate
// zero value, which matters for merging with CLI flags and defaults.
type config struct {
	Corner  *string  `yaml:"corner,omitempty"`
	Monitor *int     `yaml:"monitor,omitempty"`
	Timeout *int     `yaml:"timeout,omitempty"`
	Scale   *float64 `yaml:"scale,omitempty"`
	Padding *int     `yaml:"padding,omitempty"`
	Hours   *int     `yaml:"hours,omitempty"`
	Minutes *int     `yaml:"minutes,omitempty"`
	Seconds *int     `yaml:"seconds,omitempty"`
}

// defaultConfigPath returns ~/.config/countdown/config.yaml.
func defaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "countdown", "config.yaml"), nil
}

// loadConfig reads the config file at path. A missing file is not an error
// (it just means no preferences are set); a malformed one is fatal, since
// silently ignoring a typo'd config would be confusing.
func loadConfig(path string) config {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config{}
		}
		log.Fatalf("Failed to read config file %s: %v", path, err)
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse config file %s: %v", path, err)
	}
	return cfg
}

// writeConfig marshals cfg to YAML and writes it to path, creating the
// parent directory if needed.
func writeConfig(path string, cfg config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

type Game struct {
	startTime     time.Time
	duration      time.Duration
	windowWidth   int
	windowHeight  int
	fontFace      font.Face
	windowResized bool // To track if the window size has been set

	corner             string  // explicit corner requested via -corner, or "" to use saved/default
	monitorIdx         int     // explicit monitor requested via -monitor, or -1 to use saved/default
	resolvedMonitorIdx int     // the monitor index actually used, for persisting to state
	scale              float64 // multiplier applied to font size and padding, via -scale
	padding            int     // base padding in pixels, before -scale, via -padding

	// Placement happens in two steps, one frame apart: switching monitor
	// (via ebiten.SetMonitor) is asynchronous under tiling WMs like i3,
	// which need a moment to actually move the window before
	// ebiten.SetWindowPosition's "relative to the current monitor"
	// coordinates resolve against the right monitor.
	monitorSwitched bool
	monitorSwitchAt time.Time
	pendingX        int
	pendingY        int

	// timeout controls what happens once the countdown hits zero:
	//   0  -> close immediately
	//   <0 -> stay open (still blinking) until interrupted
	//   >0 -> stay open for that many extra seconds, then close
	timeout int
	zeroAt  time.Time // when the countdown first hit zero; zero value means it hasn't yet
}

func NewGame(duration time.Duration, fontSize float64, corner string, monitorIdx, timeout int, scale float64, padding int) *Game {
	// Use the Go Bold font embedded in golang.org/x/image so we don't depend
	// on any particular distro's font paths (e.g. DejaVu isn't guaranteed to
	// live at a Debian-style path on Arch, or be installed at all).
	tt, err := opentype.Parse(gobold.TTF)
	if err != nil {
		log.Fatalf("Failed to parse font: %v", err)
	}
	face, err := opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatalf("Failed to create font face: %v", err)
	}

	return &Game{
		startTime:     time.Now(),
		duration:      duration,
		windowWidth:   800, // Initial window width (can adjust)
		windowHeight:  200, // Initial window height (can adjust)
		fontFace:      face,
		windowResized: false,
		corner:        corner,
		monitorIdx:    monitorIdx,
		timeout:       timeout,
		scale:         scale,
		padding:       padding,
	}
}

func (g *Game) Update() error {
	if time.Since(g.startTime) < g.duration {
		return nil
	}

	if g.zeroAt.IsZero() {
		g.zeroAt = time.Now()
		fmt.Println("Countdown complete!")
	}

	switch {
	case g.timeout == 0:
		// Close immediately.
	case g.timeout < 0:
		// Stay open (still blinking) until interrupted.
		return nil
	default:
		// Stay open for `timeout` extra seconds, then close.
		if time.Since(g.zeroAt) < time.Duration(g.timeout)*time.Second {
			return nil
		}
	}

	g.savePosition()
	return ebiten.Termination
}

// savePosition persists the window's current position and monitor so the
// next run can reopen in the same spot.
func (g *Game) savePosition() {
	if !g.windowResized {
		return
	}
	x, y := ebiten.WindowPosition()
	if err := saveWindowState(x, y, g.resolvedMonitorIdx); err != nil {
		log.Printf("Failed to save window position: %v", err)
	}
}

// urgentThreshold is the point at which the countdown starts blinking
// yellow/red as it approaches zero.
const urgentThreshold = 30 * time.Second

// blinkInterval is how long each color is shown for while blinking.
const blinkInterval = 500 * time.Millisecond

var (
	colorRed    = color.RGBA{255, 0, 0, 255}
	colorYellow = color.RGBA{255, 255, 0, 255}
)

// textColor picks the countdown's color: red normally, blinking between
// yellow and red once under urgentThreshold.
func textColor(remaining time.Duration) color.RGBA {
	if remaining > urgentThreshold {
		return colorRed
	}
	if (time.Now().UnixMilli()/blinkInterval.Milliseconds())%2 == 0 {
		return colorYellow
	}
	return colorRed
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Clear the screen with transparency (or custom color for debugging)
	screen.Fill(color.RGBA{0, 0, 0, 0})

	// Calculate the remaining time. Once it hits zero, if the window is
	// staying open past the countdown (timeout != 0), count back up from
	// zero instead, prefixed with "-" to show it's overtime.
	elapsed := time.Since(g.startTime)
	remaining := g.duration - elapsed
	countingUp := remaining <= 0 && g.timeout != 0

	var countdownText string
	if countingUp {
		overtime := elapsed - g.duration
		countdownText = fmt.Sprintf("-%02d:%02d:%02d", int(overtime.Hours()), int(overtime.Minutes())%60, int(overtime.Seconds())%60)
	} else {
		if remaining < 0 {
			remaining = 0
		}
		countdownText = fmt.Sprintf("%02d:%02d:%02d", int(remaining.Hours()), int(remaining.Minutes())%60, int(remaining.Seconds())%60)
	}

	// Measure text dimensions to position correctly
	bounds, _ := font.BoundString(g.fontFace, countdownText)
	textWidth := (bounds.Max.X - bounds.Min.X).Ceil()
	textHeight := (bounds.Max.Y - bounds.Min.Y).Ceil()

	// Size and place the window only once, in two steps a frame or more
	// apart (see monitorSwitched's doc comment on Game).
	switch {
	case g.windowResized:
		// Already done.
	case !g.monitorSwitched:
		// Size the window to fit the widest text it could ever show: if
		// the window can stay open past zero (timeout != 0), that's the
		// "-HH:MM:SS" overtime form, one character wider than "HH:MM:SS".
		sizingText := countdownText
		if g.timeout != 0 && !countingUp {
			sizingText = "-" + countdownText
		}
		sizingBounds, _ := font.BoundString(g.fontFace, sizingText)
		sizingWidth := (sizingBounds.Max.X - sizingBounds.Min.X).Ceil()
		sizingHeight := (sizingBounds.Max.Y - sizingBounds.Min.Y).Ceil()

		// Resize the window to fit the text, with padding that scales
		// alongside the font so the box stays proportional at any -scale.
		padding := int(float64(g.padding) * g.scale)
		g.windowWidth = sizingWidth + padding
		g.windowHeight = sizingHeight + padding

		// Set the new window size
		ebiten.SetWindowSize(g.windowWidth, g.windowHeight)

		state, hasState := loadWindowState()
		explicit := g.corner != "" || g.monitorIdx >= 0

		monitors := ebiten.AppendMonitors(nil)
		current := ebiten.Monitor() // defaults to the current monitor
		monitor := current
		monitorIdx := 0
		for i, m := range monitors {
			if m == monitor {
				monitorIdx = i
				break
			}
		}
		if g.monitorIdx >= 0 && g.monitorIdx < len(monitors) {
			monitorIdx = g.monitorIdx
			monitor = monitors[monitorIdx]
		} else if !explicit && hasState && state.MonitorIdx < len(monitors) {
			monitorIdx = state.MonitorIdx
			monitor = monitors[monitorIdx]
		}
		g.resolvedMonitorIdx = monitorIdx

		if !explicit && hasState {
			g.pendingX, g.pendingY = state.X, state.Y
		} else {
			// Fresh placement: use the requested corner, defaulting to
			// bottom-right, with a small margin from the screen edge.
			corner := g.corner
			if corner == "" {
				corner = "bottom-right"
			}
			screenWidth, screenHeight := monitor.Size()
			g.pendingX, g.pendingY = cornerPosition(corner, screenWidth, screenHeight, g.windowWidth, g.windowHeight)
		}

		if monitor != current {
			// Actually switching monitors: under tiling WMs (e.g. i3) the
			// move is asynchronous, so defer setting the position until
			// a later frame once the window has actually landed on the
			// target monitor - otherwise SetWindowPosition below would
			// resolve "current monitor" to the old one and place the
			// window there instead.
			ebiten.SetMonitor(monitor)
			g.monitorSwitchAt = time.Now()
		} else {
			ebiten.SetWindowPosition(g.pendingX, g.pendingY)
			g.windowResized = true
		}
		g.monitorSwitched = true
	case time.Since(g.monitorSwitchAt) >= 150*time.Millisecond:
		ebiten.SetWindowPosition(g.pendingX, g.pendingY)
		g.windowResized = true
	}

	// Position for the text (centered horizontally and vertically). Ebiten's
	// text.Draw treats (x, y) as the text's baseline origin, not its
	// top-left corner, so we offset by the bounds' own origin (bounds.Min)
	// to actually center the ink itself.
	x := (g.windowWidth-textWidth)/2 - bounds.Min.X.Ceil()
	y := (g.windowHeight-textHeight)/2 - bounds.Min.Y.Ceil()

	// Draw the text on the screen
	colorRemaining := remaining
	if colorRemaining < 0 {
		colorRemaining = 0
	}
	text.Draw(screen, countdownText, g.fontFace, x, y, textColor(colorRemaining))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Return the fixed window size after resizing
	return g.windowWidth, g.windowHeight
}

func main() {
	corner := flag.String("corner", "", fmt.Sprintf("corner to place the window in (%s); defaults to the last remembered position, or bottom-right on first run", strings.Join(validCorners, ", ")))
	monitorIdx := flag.Int("monitor", -1, "index of the monitor to display on (0-based, as listed by -list-monitors); defaults to the last remembered monitor, or the current one on first run")
	listMonitors := flag.Bool("list-monitors", false, "list available monitors and exit")
	timeout := flag.Int("timeout", 0, "what to do once the countdown hits zero: 0 closes immediately, a positive value keeps blinking for that many extra seconds then closes, -1 keeps blinking until interrupted")
	scale := flag.Float64("scale", 1.0, "multiplier for the countdown's font size and box (e.g. 2 doubles it, 0.5 halves it)")
	padding := flag.Int("padding", 30, "base padding in pixels around the text, before -scale is applied")
	configPathFlag := flag.String("config", "", "path to a YAML config file with default flag values; defaults to ~/.config/countdown/config.yaml if present")
	saveConfig := flag.Bool("save-config", false, "write the current settings (flags merged with any existing config) to the config file and exit")
	var hours, mins, secs int
	flag.IntVar(&hours, "hours", 0, "hours to count down (short: -h)")
	flag.IntVar(&hours, "h", 0, "shorthand for -hours")
	flag.IntVar(&mins, "minutes", 0, "minutes to count down (short: -m)")
	flag.IntVar(&mins, "m", 0, "shorthand for -minutes")
	flag.IntVar(&secs, "seconds", 0, "seconds to count down (short: -s)")
	flag.IntVar(&secs, "s", 0, "shorthand for -seconds")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] [minutes]\n\nEither pass the countdown length as a plain number of minutes, or\nspecify it with -hours/-minutes/-seconds (or -h/-m/-s), combinable,\ne.g. -h 1 -m 30. Note -h means hours here, not help; use --help for usage.\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *listMonitors {
		for i, m := range ebiten.AppendMonitors(nil) {
			w, h := m.Size()
			fmt.Printf("%d: %s (%dx%d)\n", i, m.Name(), w, h)
		}
		return
	}

	// Fill in anything not explicitly passed on the command line from the
	// config file, so flags always take priority over saved preferences.
	setFlags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

	cfgPath := *configPathFlag
	if cfgPath == "" {
		if p, err := defaultConfigPath(); err == nil {
			cfgPath = p
		}
	}
	var cfg config
	if cfgPath != "" {
		cfg = loadConfig(cfgPath)
	}

	if !setFlags["corner"] && cfg.Corner != nil {
		*corner = *cfg.Corner
	}
	if !setFlags["monitor"] && cfg.Monitor != nil {
		*monitorIdx = *cfg.Monitor
	}
	if !setFlags["timeout"] && cfg.Timeout != nil {
		*timeout = *cfg.Timeout
	}
	if !setFlags["scale"] && cfg.Scale != nil {
		*scale = *cfg.Scale
	}
	if !setFlags["padding"] && cfg.Padding != nil {
		*padding = *cfg.Padding
	}
	if !setFlags["hours"] && !setFlags["h"] && cfg.Hours != nil {
		hours = *cfg.Hours
	}
	if !setFlags["minutes"] && !setFlags["m"] && cfg.Minutes != nil {
		mins = *cfg.Minutes
	}
	if !setFlags["seconds"] && !setFlags["s"] && cfg.Seconds != nil {
		secs = *cfg.Seconds
	}

	if *corner != "" && !slices.Contains(validCorners, *corner) {
		log.Fatalf("Invalid corner %q: must be one of %s", *corner, strings.Join(validCorners, ", "))
	}

	if *scale <= 0 {
		log.Fatalf("Invalid scale %v: must be greater than 0", *scale)
	}

	if *saveConfig {
		out := config{Timeout: timeout, Scale: scale, Padding: padding}
		if *corner != "" {
			out.Corner = corner
		}
		if *monitorIdx >= 0 {
			out.Monitor = monitorIdx
		}
		if hours > 0 {
			out.Hours = &hours
		}
		if mins > 0 {
			out.Minutes = &mins
		}
		if secs > 0 {
			out.Seconds = &secs
		}
		if cfgPath == "" {
			log.Fatal("Could not determine a config file path to save to; pass -config explicitly")
		}
		if err := writeConfig(cfgPath, out); err != nil {
			log.Fatalf("Failed to write config file %s: %v", cfgPath, err)
		}
		fmt.Printf("Saved config to %s\n", cfgPath)
		return
	}

	args := flag.Args()
	flagDuration := time.Duration(hours)*time.Hour + time.Duration(mins)*time.Minute + time.Duration(secs)*time.Second
	// Only treat hours/minutes/seconds as explicitly given if passed on the
	// command line, not merged in from the config file - otherwise a
	// config-sourced default duration would collide with a plain-minutes
	// positional arg, which should just override it.
	cliDurationGiven := setFlags["hours"] || setFlags["h"] || setFlags["minutes"] || setFlags["m"] || setFlags["seconds"] || setFlags["s"]

	var duration time.Duration
	switch {
	case cliDurationGiven && len(args) > 0:
		// Ambiguous: both an explicit CLI duration flag and a positional
		// minutes argument were given.
		flag.Usage()
		os.Exit(1)
	case len(args) == 1:
		// Backward-compatible plain-minutes form; overrides any
		// config-sourced hours/minutes/seconds.
		minutes, err := strconv.Atoi(args[0])
		if err != nil || minutes <= 0 {
			log.Fatalf("Invalid minutes: %s", args[0])
		}
		duration = time.Duration(minutes) * time.Minute
	case flagDuration > 0 && len(args) == 0:
		duration = flagDuration
	default:
		flag.Usage()
		os.Exit(1)
	}

	// Set a larger font size for better readability, scaled by -scale
	fontSize := 30.0 * *scale

	// Create a new game instance
	game := NewGame(duration, fontSize, *corner, *monitorIdx, *timeout, *scale, *padding)

	// Save the window position if the process is interrupted before the
	// countdown finishes naturally (e.g. Ctrl+C).
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		game.savePosition()
		os.Exit(0)
	}()

	// Set up the Ebiten window
	ebiten.SetWindowTitle("Countdown Overlay")
	ebiten.SetWindowResizable(false)
	ebiten.SetWindowDecorated(false) // Removes the title bar and border
	ebiten.SetWindowMousePassthrough(true)
	ebiten.SetScreenTransparent(true) // Keep the screen transparent for debugging purposes
	ebiten.SetWindowFloating(true)    // Always on top

	// Start the game loop
	if err := ebiten.RunGame(game); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}

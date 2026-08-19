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

type Game struct {
	startTime     time.Time
	duration      time.Duration
	windowWidth   int
	windowHeight  int
	fontFace      font.Face
	windowResized bool // To track if the window size has been set

	corner             string // explicit corner requested via -corner, or "" to use saved/default
	monitorIdx         int    // explicit monitor requested via -monitor, or -1 to use saved/default
	resolvedMonitorIdx int    // the monitor index actually used, for persisting to state
}

func NewGame(minutes int, fontSize float64, corner string, monitorIdx int) *Game {
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
		duration:      time.Duration(minutes) * time.Minute,
		windowWidth:   800, // Initial window width (can adjust)
		windowHeight:  200, // Initial window height (can adjust)
		fontFace:      face,
		windowResized: false,
		corner:        corner,
		monitorIdx:    monitorIdx,
	}
}

func (g *Game) Update() error {
	// Exit the game if the countdown is over
	if time.Since(g.startTime) >= g.duration {
		fmt.Println("Countdown complete!")
		g.savePosition()
		return ebiten.Termination
	}
	return nil
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

	// Calculate the remaining time
	remaining := g.duration - time.Since(g.startTime)
	if remaining < 0 {
		remaining = 0
	}
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60
	seconds := int(remaining.Seconds()) % 60

	// Format the countdown timer
	countdownText := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

	// Measure text dimensions to position correctly
	bounds, _ := font.BoundString(g.fontFace, countdownText)
	textWidth := (bounds.Max.X - bounds.Min.X).Ceil()
	textHeight := (bounds.Max.Y - bounds.Min.Y).Ceil()

	// Resize the window only once
	if !g.windowResized {
		// Resize the window to fit the text
		g.windowWidth = textWidth + 40   // Add padding (20 on each side)
		g.windowHeight = textHeight + 40 // Add padding (20 on each side)

		// Set the new window size
		ebiten.SetWindowSize(g.windowWidth, g.windowHeight)

		state, hasState := loadWindowState()
		explicit := g.corner != "" || g.monitorIdx >= 0

		monitors := ebiten.AppendMonitors(nil)
		monitor := ebiten.Monitor() // defaults to the current monitor
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
		ebiten.SetMonitor(monitor)
		g.resolvedMonitorIdx = monitorIdx

		if !explicit && hasState {
			// Restore the last known position.
			ebiten.SetWindowPosition(state.X, state.Y)
		} else {
			// Fresh placement: use the requested corner, defaulting to
			// bottom-right, with a small margin from the screen edge.
			corner := g.corner
			if corner == "" {
				corner = "bottom-right"
			}
			screenWidth, screenHeight := monitor.Size()
			x, y := cornerPosition(corner, screenWidth, screenHeight, g.windowWidth, g.windowHeight)
			ebiten.SetWindowPosition(x, y)
		}

		// Mark the window as resized
		g.windowResized = true
	}

	// Position for the text (centered horizontally and vertically)
	x := (g.windowWidth - textWidth) / 2
	y := (g.windowHeight-textHeight)/2 + 20

	// Draw the text on the screen
	text.Draw(screen, countdownText, g.fontFace, x, y, textColor(remaining))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Return the fixed window size after resizing
	return g.windowWidth, g.windowHeight
}

func main() {
	corner := flag.String("corner", "", fmt.Sprintf("corner to place the window in (%s); defaults to the last remembered position, or bottom-right on first run", strings.Join(validCorners, ", ")))
	monitorIdx := flag.Int("monitor", -1, "index of the monitor to display on (0-based, as listed by -list-monitors); defaults to the last remembered monitor, or the current one on first run")
	listMonitors := flag.Bool("list-monitors", false, "list available monitors and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <minutes>\n\nFlags:\n", os.Args[0])
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

	if *corner != "" && !slices.Contains(validCorners, *corner) {
		log.Fatalf("Invalid corner %q: must be one of %s", *corner, strings.Join(validCorners, ", "))
	}

	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	// Convert the argument to an integer
	minutes, err := strconv.Atoi(args[0])
	if err != nil || minutes <= 0 {
		log.Fatalf("Invalid minutes: %s", args[0])
	}

	// Set a larger font size for better readability
	fontSize := 30.0

	// Create a new game instance
	game := NewGame(minutes, fontSize, *corner, *monitorIdx)

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

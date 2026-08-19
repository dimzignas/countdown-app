package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
)

// windowState is persisted across runs so the overlay reopens where it was left.
type windowState struct {
	X int `json:"x"`
	Y int `json:"y"`
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

func saveWindowState(x, y int) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(windowState{X: x, Y: y})
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
}

func NewGame(minutes int, fontSize float64) *Game {
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
		startTime:    time.Now(),
		duration:     time.Duration(minutes) * time.Minute,
		windowWidth:  800, // Initial window width (can adjust)
		windowHeight: 200, // Initial window height (can adjust)
		fontFace:     face,
		windowResized: false,
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

// savePosition persists the window's current position so the next run
// can reopen in the same spot.
func (g *Game) savePosition() {
	if !g.windowResized {
		return
	}
	x, y := ebiten.WindowPosition()
	if err := saveWindowState(x, y); err != nil {
		log.Printf("Failed to save window position: %v", err)
	}
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
		g.windowWidth = textWidth + 40  // Add padding (20 on each side)
		g.windowHeight = textHeight + 40 // Add padding (20 on each side)

		// Set the new window size
		ebiten.SetWindowSize(g.windowWidth, g.windowHeight)

		if state, ok := loadWindowState(); ok {
			// Restore the last known position.
			ebiten.SetWindowPosition(state.X, state.Y)
		} else {
			// No saved position yet: default to the bottom-right corner.
			screenWidth, screenHeight := ebiten.Monitor().Size()
			ebiten.SetWindowPosition(screenWidth-g.windowWidth-10, screenHeight-g.windowHeight-10) // 10px margin from bottom-right corner
		}

		// Mark the window as resized
		g.windowResized = true
	}

	// Position for the text (centered horizontally and vertically)
	x := (g.windowWidth - textWidth) / 2
	y := (g.windowHeight - textHeight) / 2 + 20

	// Draw the text on the screen
	text.Draw(screen, countdownText, g.fontFace, x, y, color.RGBA{255, 0, 0, 255})
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Return the fixed window size after resizing
	return g.windowWidth, g.windowHeight
}

func main() {
	// Get the minutes argument from the command line
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <minutes>", os.Args[0])
	}

	// Convert the argument to an integer
	minutes, err := strconv.Atoi(os.Args[1])
	if err != nil || minutes <= 0 {
		log.Fatalf("Invalid minutes: %s", os.Args[1])
	}

	// Set a larger font size for better readability
	fontSize := 30.0

	// Create a new game instance
	game := NewGame(minutes, fontSize)

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

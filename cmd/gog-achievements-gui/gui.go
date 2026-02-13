package main

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/timendum/gog-achievements/internal"

	// to decode image
	_ "golang.org/x/image/webp"
)

// ===============================
// UI game card state (gameItem)
// ===============================
// gameItem represents the UI state for a single game card displayed in the grid.
// It manages async loading of game details (title, image URL) and image rendering.
// State is protected by a mutex for safe concurrent access during async operations.

type gameItem struct {
	id int

	// click widget manages clickable area state for this card
	click widget.Clickable

	// === Protected by mu - safe for concurrent access during async loading ===
	mu            sync.RWMutex
	title         string        // game title; initially the id string; replaced when details load
	detailLoaded  bool          // flag: game details (title, image URL) have been fetched
	detailLoading bool          // flag: detail fetch is currently in progress
	imageURL      string        // URL to game's promotional image for display
	imgLoaded     bool          // flag: image has been decoded and is ready for rendering
	imgLoading    bool          // flag: image download/decode is in progress
	imgErr        error         // stores any error from image fetch/decode for debugging
	imgOp         paint.ImageOp // gioui paint operation containing the decoded image data for rendering
	highlighted   bool          // flag
}

// newGameItem creates a new empty game card state.
// Initially displays the game ID as title until details are loaded asynchronously.
func newGameItem(id int) *gameItem {
	return &gameItem{
		id:    id,
		title: fmt.Sprintf("%d", id), // placeholder: show game ID until detail fetch completes
	}
}

// ===== Thread-safe accessors for UI rendering =====
// These methods safely read gameItem state with mutex protection.

// Title returns the current game title for display (thread-safe read).
func (g *gameItem) Title() string { g.mu.RLock(); defer g.mu.RUnlock(); return g.title }

// HasImage checks if image is ready for rendering (thread-safe read).
func (g *gameItem) HasImage() bool { g.mu.RLock(); defer g.mu.RUnlock(); return g.imgLoaded }

// ImageOp returns the paint operation for gioui rendering the game image (thread-safe read).
func (g *gameItem) ImageOp() paint.ImageOp {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.imgOp
}

// ===============================================
// Async Loading Pipeline: Details → Image → Render
// ===============================================
// GameItems load data asynchronously in two stages:
// Stage 1: Fetch game details (title, image URL) via detailsSem-limited goroutine
// Stage 2: Download image data via imagesSem-limited goroutine when URL arrives
// Each completion triggers window.Invalidate() to request UI rerender.

// detailsSem limits concurrent detail fetches to 6 to avoid overwhelming the API.
var detailsSem = make(chan struct{}, 6)

// imagesSem limits concurrent image downloads to 8 to bound network/memory usage.
var imagesSem = make(chan struct{}, 8)

// loadGameDetail starts an async fetch of game details (title, image URL).
// Returns immediately; actual fetch runs in a spawned goroutine with semaphore limiting.
// Called every frame from gameCard(); early returns prevent redundant fetches.
func (g *gameItem) loadGameDetail(w *app.Window, get func(int) *internal.GameDetail) {
	g.mu.Lock()
	if g.detailLoaded || g.detailLoading {
		g.mu.Unlock()
		return // already loaded or in progress
	}
	g.detailLoading = true
	g.mu.Unlock()

	go func(id int) {
		// Semaphore: acquire slot, fetch detail, release slot
		detailsSem <- struct{}{}
		defer func() { <-detailsSem }()

		// Fetch game details from backend
		d := get(id)
		if d == nil {
			return
		}

		// Update gameItem state with fetched data
		g.mu.Lock()
		g.title = d.Title
		g.imageURL = fmt.Sprintf("https://images.gog-statics.com/%s_product_tile_extended_432x243.webp", d.ImageLogo)
		g.detailLoaded = true
		g.detailLoading = false
		// reset image state in case details changed
		g.imgLoaded = false
		g.imgLoading = false
		g.imgErr = nil
		g.mu.Unlock()

		// Signal gioui to redraw UI with updated title
		w.Invalidate()

		// Kick off image download pipeline as soon as URL is available
		if g.imageURL != "" {
			g.downloadPrintImage(w)
		}
	}(g.id)
}

// downloadPrintImage starts an async download and decode of the game image.
// Returns immediately; actual download/decode runs in a spawned goroutine with semaphore limiting.
// Called every frame from gameCard(); early returns prevent redundant fetches.
func (g *gameItem) downloadPrintImage(w *app.Window) {
	g.mu.Lock()
	if g.imgLoaded || g.imgLoading || g.imageURL == "" {
		g.mu.Unlock()
		return // already loaded, in progress, or no URL yet
	}
	url := g.imageURL
	g.imgLoading = true
	g.mu.Unlock()

	go func(u string) {
		// Semaphore: acquire slot, download/decode image, release slot
		imagesSem <- struct{}{}
		defer func() { <-imagesSem }()

		// Download and decode image from URL
		img, err := fetchImage(u)

		g.mu.Lock()
		defer g.mu.Unlock()
		if err != nil {
			g.imgErr = err
			g.imgLoading = false
			w.Invalidate() // signal rerender to show placeholder
			return
		}
		// Convert decoded image into a gioui paint operation for rendering
		g.imgOp = paint.NewImageOp(img)
		g.imgLoaded = true
		g.imgLoading = false
		g.imgErr = nil

		// Signal gioui to redraw UI with the new image
		w.Invalidate()
	}(url)
}

// Simple HTTP image fetch with timeout.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// Download an image, parse and return it
func fetchImage(url string) (image.Image, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return nil, io.ErrUnexpectedEOF
	}
	img, _, err := image.Decode(resp.Body)
	return img, err
}

// ===============================================
// Main App Entry Point
// ===============================================
// main launches the gioui event loop via app.Main(),
// while runUI() executes in a separate goroutine to build and render the UI.

func main() {
	go runUI() // start UI rendering goroutine
	app.Main() // start gioui framework and event loop (blocks)
}

// runUI initializes the gioui Window and auth, then runs the main event loop.
// Window events (resize, frame requests) are processed and rendered continuously.
func runUI() {
	// Create gioui window
	w := new(app.Window)

	// Configure window properties
	w.Option(app.Title("Gog Achievements Manager"))
	w.Option(app.Size(unit.Dp(1000), unit.Dp(700)))

	// Load GOG data
	refreshToken, err := internal.GetRefreshToken()
	if err != nil {
		fmt.Printf("Failed to get refresh token: %v\n", err)
		return
	}

	if refreshToken == "" {
		fmt.Println("No refresh token found in registry.")
		return
	}
	authResp, err := internal.GetAuth(refreshToken, "", "")
	if err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		return
	}

	if authResp.RefreshToken == "" {
		fmt.Println("No refresh token in response")
		return
	}

	if authResp.RefreshToken == "" {
		fmt.Println("No refresh token in response")
		return
	}
	games := internal.ListOwnedGameIDs(*authResp)
	items := make([]*gameItem, 0, len(*games))
	for _, id := range *games {
		items = append(items, newGameItem(id))
	}

	// ops: reusable buffer for recording all gioui drawing operations each frame
	var ops op.Ops
	// th: material design theme used to style all text and widgets
	th := material.NewTheme()

	// rowList: scrollable vertical container for game card rows
	// Each row contains multiple card columns depending on window width
	var rowList widget.List
	rowList.Axis = layout.Vertical

	var searchEditor widget.Editor
	searchEditor.SingleLine = true
	var searchText string
	// last index of games matched for search
	searchLastIdx := -1
	var searchNextButton widget.Clickable

	for {
		// ===== Main Event Loop =====
		// Process gioui events (window lifecycle, frame requests, user input)
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			// Window is closing; clean exit
			if e.Err != nil {
				log.Println("Error:", e.Err)
			}
			return

		case app.FrameEvent:
			// Frame event: window needs rendering (resize, invalidation, animation frame, etc.)
			// gtx: gioui context for this frame; records operations into ops buffer
			gtx := app.NewContext(&ops, e)

			// ===== Grid Layout Configuration =====
			// Responsive grid: cards rearrange based on window width
			const (
				cardWidthDp = 220 // fixed card width (device pixels)
				cardGapDp   = 12  // horizontal spacing between cards
				rowGapDp    = 12  // vertical spacing between rows
				imgHeightDp = 120 // height of game image in card
				cardPadDp   = 8   // padding inside card (not used in current layout)
				badgePadDp  = 6   // badge padding (not used in current layout)
				radiusDp    = 8   // border radius for card corners
			)

			// Convert device-independent pixels (Dp) to pixels based on screen DPI
			cardWpx := gtx.Dp(unit.Dp(cardWidthDp))
			cardGapPx := gtx.Dp(unit.Dp(cardGapDp))

			// Calculate responsive grid: how many card columns fit in available width?
			avail := max(gtx.Constraints.Max.X, 1)                                   // available width in pixels
			cols := max(int(float32(avail+cardGapPx)/float32(cardWpx+cardGapPx)), 1) // columns per row
			n := len(*games)                                                         // total games
			rows := max(int(math.Ceil(float64(n)/float64(cols))), 1)                 // rows needed to fit all games

			if searchEditor.Text() != searchText {
				searchText = searchEditor.Text()
				searchLastIdx = -1
				if searchText != "" {
					for i, item := range items {
						if strings.Contains(strings.ToLower(item.Title()), strings.ToLower(searchText)) {
							rowList.Position.First = i / cols
							rowList.Position.Offset = 0
							searchLastIdx = i
							break
						}
					}
				}
			}
			if searchNextButton.Clicked(gtx) && searchText != "" {
				searchLastIdx = searchLastIdx + 1
				for i := range items {
					item := items[(i+searchLastIdx)%n]
					if strings.Contains(strings.ToLower(item.Title()), strings.ToLower(searchText)) {
						//real idx on games and items
						i = (searchLastIdx + i) % n
						rowList.Position.First = i / cols
						rowList.Position.Offset = 0
						searchLastIdx = i
						break
					}
				}
			}

			// Convert rowList to a material.List for styled scrolling with material theme
			mList := material.List(th, &rowList)
			mList.AnchorStrategy = material.Overlay // keep scroll position stable during updates

			// Grey row
			layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Background{}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
							paint.Fill(gtx.Ops, color.NRGBA{R: 200, G: 200, B: 200, A: 255})
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Min.Y)}
						},
						func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return material.Body1(th, "Search:").Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return material.Editor(th, &searchEditor, "").Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return material.Button(th, &searchNextButton, "Next").Layout(gtx)
									}),
								)
							})
						},
					)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						// mList.Layout renders all rows; calls rowHeight callback for each visible/needed row
						return mList.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, func() []layout.FlexChild {
										children := make([]layout.FlexChild, 0, cols*2-1)
										// Build FlexChild array alternating cards and gaps
										// Pattern: [Card1, Gap, Card2, Gap, Card3, ...] (no gap after last column)
										for c := range cols {
											idx := row*cols + c // linear index into items array
											if idx >= n {
												// Game slot beyond total count: add empty spacer to maintain row alignment
												children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWpx, cardWpx
													return layout.Dimensions{Size: image.Pt(cardWpx, 0)}
												}))
											} else {
												it := items[idx]
												it.highlighted = (idx == searchLastIdx)
												// Render gameCard with fixed width
												children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													// Lock card width; height determined by gameCard content
													gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWpx, cardWpx
													return gameCard(th, w, it,
														unit.Dp(cardPadDp),
														unit.Dp(imgHeightDp),
														unit.Dp(badgePadDp),
														unit.Dp(radiusDp),
													)(gtx)
												}))
											}
											// Add horizontal gap between cards (except after last column)
											if c != cols-1 {
												children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return layout.Spacer{Width: unit.Dp(cardGapDp)}.Layout(gtx)
												}))
											}
										}
										return children
									}()...)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									dims := layout.Spacer{Height: unit.Dp(rowGapDp)}.Layout(gtx)
									defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
									paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
									return dims
								}),
							)
						})
					})
				}),
			)

			// Flush ops buffer: render all recorded drawing operations to the window
			e.Frame(gtx.Ops)
		}
	}
}

// ===============================================
// Game Card Widget
// ===============================================
// gameCard builds a single game card using gioui layout and paint primitives.
// Returns a layout.Widget function that renders the card and manages its clickable state.

func gameCard(th *material.Theme, w *app.Window, it *gameItem,
	pad unit.Dp, imgHeight unit.Dp, _ unit.Dp, radius unit.Dp) layout.Widget {

	return func(gtx layout.Context) layout.Dimensions {
		// === Async Loading Pipeline ===
		// Stage 1: Trigger detail load (title, image URL)
		it.loadGameDetail(w, internal.GetGameDetail)
		// Stage 2: Trigger image download (happens after details arrive)
		it.downloadPrintImage(w)

		// === Click Handling ===
		// Check if user clicked this card
		for it.click.Clicked(gtx) {
			log.Println("Clicked:", it.id, it.Title())
		}

		// === Card Background ===
		// Draw light gray rounded rectangle as card background
		r := gtx.Dp(radius)
		defer clip.RRect{
			Rect: image.Rectangle{Max: gtx.Constraints.Max}, // fill entire card area
			NE:   r, NW: r, SE: r, SW: r,                    // rounded corners
		}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, color.NRGBA{R: 225, G: 225, B: 225, A: 255})

		// === Card Content & Interaction ===
		return it.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Wrap all content with padding
			return layout.UniformInset(pad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Vertical layout: image on top, title below
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// ===== Image Area =====
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Fix image height
						h := gtx.Dp(imgHeight)
						gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h

						// Stack: renders either the image or placeholder
						return layout.Stack{}.Layout(gtx,
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								if it.HasImage() {
									// Image is ready: render it with cover fit (crop to fill)
									img := widget.Image{Src: it.ImageOp(), Fit: widget.Cover}
									return img.Layout(gtx)
								}
								// Image not loaded yet: show medium gray placeholder
								paint.Fill(gtx.Ops, color.NRGBA{R: 200, G: 200, B: 200, A: 255})
								return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, h)}
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					// ===== Title Text =====
					// Displays game title; updates when details async-load arrives
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Create styled body text from theme
						t := material.Body1(th, it.Title())
						if it.highlighted {
							t = material.Body1(th, fmt.Sprintf("*%s*", it.Title()))
						}
						t.MaxLines = 2 // truncate long titles to 2 lines
						return t.Layout(gtx)
					}),
				)
			})
		})
	}
}

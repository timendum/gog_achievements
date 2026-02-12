package main

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"math"
	"net/http"
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

// ------------------------------
// UI game card state
// ------------------------------

type gameItem struct {
	id int

	click widget.Clickable

	// guarded by mu
	mu            sync.RWMutex
	title         string // initially the id; replaced by detail
	detailLoaded  bool
	detailLoading bool
	imageURL      string
	imgLoaded     bool
	imgLoading    bool
	imgErr        error
	imgOp         paint.ImageOp
}

// Create an new GameItem
func newGameItem(id int) *gameItem {
	return &gameItem{
		id:    id,
		title: fmt.Sprintf("%d", id), // show something at startup
	}
}

//helpers

func (g *gameItem) Title() string  { g.mu.RLock(); defer g.mu.RUnlock(); return g.title }
func (g *gameItem) HasImage() bool { g.mu.RLock(); defer g.mu.RUnlock(); return g.imgLoaded }
func (g *gameItem) ImageOp() paint.ImageOp {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.imgOp
}

/*
   =========================================
   Async loading: details -> image
   =========================================
*/

// Limit concurrent network work (both details and images).
var detailsSem = make(chan struct{}, 6)
var imagesSem = make(chan struct{}, 8)

// Async function to load game details
func (g *gameItem) loadGameDetail(w *app.Window, get func(int) *internal.GameDetail) {
	g.mu.Lock()
	if g.detailLoaded || g.detailLoading {
		g.mu.Unlock()
		return
	}
	g.detailLoading = true
	g.mu.Unlock()

	go func(id int) {
		// Concurrency limit
		detailsSem <- struct{}{}
		defer func() { <-detailsSem }()

		d := get(id)
		if d == nil {
			return
		}

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

		// Ask Gio to redraw
		w.Invalidate()

		// As soon as details arrive, begin image load (if URL supplied)
		if g.imageURL != "" {
			g.downloadPrintImage(w)
		}
	}(g.id)
}

// Async function to download and print the image
func (g *gameItem) downloadPrintImage(w *app.Window) {
	g.mu.Lock()
	if g.imgLoaded || g.imgLoading || g.imageURL == "" {
		g.mu.Unlock()
		return
	}
	url := g.imageURL
	g.imgLoading = true
	g.mu.Unlock()

	go func(u string) {
		// semaphore
		imagesSem <- struct{}{}
		defer func() { <-imagesSem }()

		img, err := fetchImage(u)

		g.mu.Lock()
		defer g.mu.Unlock()
		if err != nil {
			g.imgErr = err
			g.imgLoading = false
			w.Invalidate()
			return
		}
		g.imgOp = paint.NewImageOp(img)
		g.imgLoaded = true
		g.imgLoading = false
		g.imgErr = nil

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

// ------------------------------
// App
// ------------------------------

func main() {
	go runUI()
	app.Main()
}

func runUI() {
	window := new(app.Window)

	window.Option(app.Title("Gog Achievements Manager"))
	window.Option(app.Size(unit.Dp(1000), unit.Dp(700)))

	var ops op.Ops
	th := material.NewTheme()

	// Scrollable list of "rows"; each row will contain N cards (columns).
	var rowList widget.List
	rowList.Axis = layout.Vertical

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

	for {
		// Main GUI Loop
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			if e.Err != nil {
				log.Println("Error:", e.Err)
			}
			return

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			// --- Grid tuning ---
			const (
				cardWidthDp = 220
				cardGapDp   = 12
				rowGapDp    = 12
				imgHeightDp = 120
				cardPadDp   = 8
				badgePadDp  = 6
				radiusDp    = 8
			)

			cardWpx := gtx.Dp(unit.Dp(cardWidthDp))
			cardGapPx := gtx.Dp(unit.Dp(cardGapDp))
			// rowGapPx := gtx.Dp(unit.Dp(rowGapDp))

			// Figure out how many columns fit
			avail := gtx.Constraints.Max.X
			if avail == 0 {
				avail = 1
			}
			cols := int(float32(avail+cardGapPx) / float32(cardWpx+cardGapPx))
			if cols < 1 {
				cols = 1
			}
			n := len(*games)
			rows := int(math.Ceil(float64(n) / float64(cols)))
			if rows < 1 {
				rows = 1
			}

			// Scrollable list of rows
			mList := material.List(th, &rowList)
			mList.AnchorStrategy = material.Overlay // default fine

			layout.Inset{Top: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return mList.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
					// add gap between rows
					defer func() {
						if row != rows-1 {
							// spacer below row
							layout.Spacer{Height: unit.Dp(unit.Dp(rowGapDp))}.Layout(gtx)
						}
					}()

					// Row layout: horizontally place up to `cols` cards
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, func() []layout.FlexChild {
						children := make([]layout.FlexChild, 0, cols*2-1)
						for c := 0; c < cols; c++ {
							idx := row*cols + c
							if idx >= n {
								// Fill remaining space with empty rigid so row stays aligned
								children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWpx, cardWpx
									return layout.Dimensions{Size: image.Pt(cardWpx, 0)}
								}))
							} else {
								it := items[idx]
								children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									// fix width for each card
									gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWpx, cardWpx
									return gameCard(th, window, it,
										unit.Dp(cardPadDp),
										unit.Dp(imgHeightDp),
										unit.Dp(badgePadDp),
										unit.Dp(radiusDp),
									)(gtx)
								}))
							}
							// column gap (not after last column)
							if c != cols-1 {
								children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{Width: unit.Dp(cardGapDp)}.Layout(gtx)
								}))
							}
						}
						return children
					}()...)
				})
			})

			e.Frame(gtx.Ops)
		}
	}
}

// gameCard builds a clickable card using only Gio primitives.

func gameCard(th *material.Theme, w *app.Window, it *gameItem,
	pad unit.Dp, imgHeight unit.Dp, _ unit.Dp, radius unit.Dp) layout.Widget {

	return func(gtx layout.Context) layout.Dimensions {
		// Stage 1: ensure details; Stage 2: ensure image (detail loader also triggers it).
		it.loadGameDetail(w, internal.GetGameDetail)
		it.downloadPrintImage(w)

		// Handle clicks
		for it.click.Clicked(gtx) {
			log.Println("Clicked:", it.id, it.Title())
		}

		// Rounded background
		r := gtx.Dp(radius)
		defer clip.RRect{
			Rect: image.Rectangle{Max: gtx.Constraints.Max},
			NE:   r, NW: r, SE: r, SW: r,
		}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, color.NRGBA{R: 25, G: 25, B: 25, A: 255})

		return it.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(pad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// ---- Image area ----
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						h := gtx.Dp(imgHeight)
						gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h

						return layout.Stack{}.Layout(gtx,
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								if it.HasImage() {
									img := widget.Image{Src: it.ImageOp(), Fit: widget.Cover}
									return img.Layout(gtx)
								}
								// placeholder background while loading
								paint.Fill(gtx.Ops, color.NRGBA{R: 40, G: 40, B: 40, A: 255})
								return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, h)}
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					// ---- Title (updates when details arrive) ----
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						t := material.Body1(th, it.Title())
						t.MaxLines = 2
						return t.Layout(gtx)
					}),
				)
			})
		})
	}
}

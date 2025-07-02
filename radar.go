package main

import (
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// radarContext holds geometry and sizing for radar rendering
type radarContext struct {
	cx, cy int
	width  int
	height int
	maxR   float64
	r      float64
}

func inBounds(width int, height int, x int, y int) bool {
	if x >= 0 && x < width && y >= 0 && y < height {
		return true
	}
	return false
}

func getPlaneSymbol(p plane) rune {
	heading := p.Heading

	if heading >= 315 || heading < 45 {
		return '^'
	}
	if heading >= 45 && heading < 135 {
		return '>'
	}
	if heading >= 135 && heading < 225 {
		return 'v'
	}
	if heading >= 225 && heading < 315 {
		return '<'
	}
	return '*'
}

func (m *model) renderDistanceLabels(ctx radarContext) {
	charsPerNM := float64(ctx.r) / float64(m.radarRange)
	minLabelSpacingPx := 3.0
	maxLabels := int(float64(ctx.r) / (minLabelSpacingPx * charsPerNM))
	if maxLabels < 1 {
		maxLabels = 1
	}
	niceSteps := []float64{1, 5, 10, 15, 20, 25, 50, 100, 200, 500}
	labelStepNM := niceSteps[len(niceSteps)-1]
	for _, step := range niceSteps {
		if float64(m.radarRange)/step <= float64(maxLabels) {
			labelStepNM = step
			break
		}
	}
	for d := labelStepNM; d < float64(m.radarRange); d += labelStepNM {
		radius := d * charsPerNM
		x := ctx.cx + int(radius*math.Sin(0))
		y := ctx.cy - int(radius*math.Cos(0)*m.aspectRatio)
		label := strconv.Itoa(int(d))
		for i, r := range []rune(label) {
			if inBounds(ctx.width, ctx.height, x+i, y) {
				c := &m.buffer[y][x+i]
				c.kind = "label"
				c.char = r
			}
		}
	}
}

func (m *model) renderSweepArm(ctx radarContext) {
	prevAngle := m.sweepAngle - 0.15
	for l := 0; l <= int(ctx.r); l++ {
		for interp := 0.0; interp <= 1.0; interp += 0.1 {
			theta := prevAngle + interp*(m.sweepAngle-prevAngle)
			x := ctx.cx + int(float64(l)*math.Sin(theta))
			y := ctx.cy - int(float64(l)*math.Cos(theta)*m.aspectRatio)
			if inBounds(len(m.buffer[0]), len(m.buffer), x, y) {
				c := &m.buffer[y][x]
				c.kind = "sweep"
				c.sweepAge = int(interp * 10)
				c.char = ' '
			}
		}
	}
}

func (m *model) renderPlanes(ctx radarContext) {
	for _, p := range m.planes {
		if _, ok := m.visiblePlanes[p.FlightCode]; ok {
			if p.DistanceFromObserver > float64(m.radarRange) {
				continue
			}
			scale := float64(ctx.maxR-4) / float64(m.radarRange)
			virtualDistance := p.DistanceFromObserver * scale
			displayBearing := p.BearingFromObserver - m.northOffset
			posX := ctx.cx + int(virtualDistance*math.Sin(displayBearing))
			posY := ctx.cy - int(virtualDistance*math.Cos(displayBearing)*m.aspectRatio)
			dx := float64(posX - ctx.cx)
			dy := float64(posY - ctx.cy)
			if inBounds(ctx.width, ctx.height, posX, posY) && math.Sqrt(dx*dx+dy*dy) < ctx.r {
				c := &m.buffer[posY][posX]
				c.kind = "plane"
				c.char = getPlaneSymbol(p)
				c.sweepAge = 0
			}
		}
	}
}

func (m *model) renderBearingLabels(ctx radarContext) {
	tickMarks := []float64{0, 90, 180, 270}
	tickLabels := [][]rune{
		{'0'},
		{'9', '0'},
		{'1', '8', '0'},
		{'2', '7', '0'},
	}
	for i, tick := range tickMarks {
		phi := tick * math.Pi / 180
		phi -= m.northOffset
		x := ctx.cx + int((ctx.r+3)*math.Sin(phi))
		y := ctx.cy - int((ctx.r+3)*math.Cos(phi)*m.aspectRatio)
		for j, r := range tickLabels[i] {
			if inBounds(ctx.width, ctx.height, x+j, y) {
				c := &m.buffer[y][x+j]
				c.char = r
				c.kind = "label"
			}
		}
	}
}

func (m *model) renderRadar(width, height int) string {
	if width < 5 || height < 5 || len(m.buffer) < 5 || len(m.buffer[0]) < 5 {
		return "Too small"
	}

	// Clear the buffer
	for y := range m.buffer {
		for x := range m.buffer[y] {
			c := &m.buffer[y][x]
			if c.kind != "plane" {
				c.char = ' '
				c.kind = "blank"
			}
		}
	}

	// Radar radius is bounded by the width and height of the terminal -5 for padding
	maxRx := float64(width/2) - 5
	maxRy := float64(height)/(2.0*m.aspectRatio) - 5
	maxR := min(maxRx, maxRy)
	r := maxR

	cx := width / 2
	cy := height / 2

	ctx := radarContext{
		cx:     cx,
		cy:     cy,
		width:  width,
		height: height,
		maxR:   maxR,
		r:      r,
	}

	m.renderDistanceLabels(ctx)
	m.renderSweepArm(ctx)
	m.renderPlanes(ctx)
	m.renderBearingLabels(ctx)

	var b strings.Builder
	for _, row := range m.buffer {
		for _, c := range row {
			if c.kind == "ring" {
				b.WriteString(frameBg.Render(" "))
				continue
			}
			style := lipgloss.NewStyle()
			// Color fades based on how long ago it was sweeped
			switch {
			case c.sweepAge <= 2:
				style = style.Background(brightGreen)
			case c.sweepAge > 2 && c.sweepAge <= 7:
				style = style.Background(mediumGreen)
			case c.sweepAge > 3 && c.sweepAge <= 12:
				style = style.Background(dimGreen)
			}
			// Color the plane icons based on how long ago it was sweeped. Takes longer to fade than the background.
			if c.kind == "plane" {
				switch {
				case c.sweepAge <= 15:
					style = style.Foreground(brightGreen)
				case c.sweepAge > 15 && c.sweepAge <= 30:
					style = style.Foreground(mediumGreen)
				case c.sweepAge > 30 && c.sweepAge <= 60:
					style = style.Foreground(dimGreen)
				case c.sweepAge > 60 && c.sweepAge <= 90:
					style = style.Foreground(dimmestGreen)
				case c.sweepAge == 99:
					c.kind = "blank"
					c.char = ' '
				default:
					style = style.Foreground(dimGreen)
				}
				b.WriteString(style.Render(string(c.char)))
				continue
			}
			b.WriteString(style.Render(string(c.char)))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

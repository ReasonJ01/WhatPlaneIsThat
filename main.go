package main

import (
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/umahmood/haversine"
)

var brightGreen = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00"))
var mediumGreen = lipgloss.NewStyle().Foreground(lipgloss.Color("#009900"))
var dimGreen = lipgloss.NewStyle().Foreground(lipgloss.Color("#003300"))

var brightGreenBg = lipgloss.NewStyle().Background(lipgloss.Color("#00ff00"))
var mediumGreenBg = lipgloss.NewStyle().Background(lipgloss.Color("#009900"))
var dimGreenBg = lipgloss.NewStyle().Background(lipgloss.Color("#003300"))
var emptyBg = lipgloss.NewStyle().Background(lipgloss.Color("#1b1c1c"))
var frameBg = lipgloss.NewStyle().Background(lipgloss.Color("#3b3a3a"))

type plane struct {
	lat                  float64
	lon                  float64
	bearingFromObserver  float64
	distanceFromObserver float64
}

type model struct {
	width       int
	height      int
	maxR        float64
	aspectRatio float64
	sweepAngle  float64
	northOffset float64
	radarRange  int
	buffer      [][]cell
	planes      []plane
	lat         float64
	lon         float64
}

type cell struct {
	char     rune
	kind     string
	sweepAge int
}

func (m model) Init() tea.Cmd {
	return doTick()

}

type tickMsg time.Time

func doTick() tea.Cmd {
	return tea.Tick(time.Millisecond*200, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "=":
			m.northOffset += 0.1
			return m, nil

		case "-":
			m.northOffset -= 0.1
			return m, nil

		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		maxRx := float64(m.width / 4)
		maxRy := float64(m.height) / (2.0 * m.aspectRatio)
		m.maxR = min(maxRx, maxRy)
		m.buffer = make([][]cell, m.height)
		for y := range m.buffer {
			m.buffer[y] = make([]cell, m.width)
			for x := range m.buffer[y] {
				m.buffer[y][x] = cell{' ', "blank", int(100)}
			}
		}
		return m, nil

	case tickMsg:
		m.sweepAngle += 0.1
		if m.sweepAngle >= 2*math.Pi {
			m.sweepAngle = 0
		}

		for y := range m.buffer {
			for x := range m.buffer[y] {
				c := &m.buffer[y][x]
				if c.sweepAge < 100 {
					c.sweepAge = c.sweepAge + 1
				}

			}
		}

		m.planes = make([]plane, 1)
		m.planes[0] = plane{0, 0.5, 0, 0}
		m.SetPlaneLocationDetails(&m.planes[0])

		return m, doTick()
	}

	return m, nil
}

func inBounds(width int, height int, x int, y int) bool {
	if x > 0 && x < width && y > 0 && y < height {
		return true
	}
	return false
}

func getOutlineChar(theta float64) rune {
	if theta < math.Pi/2 {
		return '/'
	}
	if theta < math.Pi {
		return '\\'
	}

	if theta < 3*math.Pi/2 {
		return '/'
	}

	return '\\'

}

func (m model) SetPlaneLocationDetails(p *plane) {
	curr_location := haversine.Coord{Lat: m.lat, Lon: m.lon}
	planeLocation := haversine.Coord{Lat: p.lat, Lon: p.lon}

	mi, _ := haversine.Distance(curr_location, planeLocation)
	nm := mi / 1.15078
	scale := float64(m.maxR-4) / float64(m.radarRange)
	virtualDistance := nm * scale
	p.distanceFromObserver = virtualDistance

	dLon := p.lon - m.lon
	y := math.Sin(dLon) * math.Cos(p.lat)
	x := math.Cos(m.lat)*math.Sin(p.lat) - math.Sin(m.lat)*math.Cos(p.lat)*math.Cos(dLon)
	bearing := math.Atan2(y, x)
	if bearing < 0 {
		bearing += 2 * math.Pi
	}

}

func withinSweep(bearing, sweepAngle, width float64) bool {
	dTheta := bearing - sweepAngle

	return dTheta <= 0.5

}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	r := m.maxR - 4
	cx := int(m.maxR)
	cy := (m.height + 2) / 2

	charsPerNM := float64(r) / float64(m.radarRange)

	var labelStepNM float64
	switch {
	case charsPerNM >= 2:
		labelStepNM = 10
	case charsPerNM >= 1:
		labelStepNM = 20
	default:
		labelStepNM = 20
	}

	for y := range m.buffer {
		for x := range m.buffer[y] {
			c := &m.buffer[y][x]
			//r := []rune(strconv.Itoa(c.sweepAge))
			c.char = ' '
			c.kind = "blank"
		}
	}

	// Distance Labels
	for d := labelStepNM; d < float64(m.radarRange); d += labelStepNM {
		radius := d * charsPerNM
		x := cx + int(radius*math.Sin(0))
		y := cy - int(radius*math.Cos(0)*m.aspectRatio)

		label := strconv.Itoa(int(d))
		for i, r := range []rune(label) {
			if inBounds(m.width, m.height, x+i, y) {
				c := &m.buffer[y][x+i]
				c.kind = "label"
				c.char = r
			}
		}
	}

	for _, p := range m.planes {
		if withinSweep(p.bearingFromObserver, m.sweepAngle, 0.8) {
			posX := cx + int(p.distanceFromObserver*math.Cos(p.bearingFromObserver+float64(m.northOffset)))
			posY := cy + int(p.distanceFromObserver*math.Sin(p.bearingFromObserver+float64(m.northOffset))*m.aspectRatio)

			if inBounds(m.width, m.height, posX, posY) {
				c := &m.buffer[posY][posX]
				c.kind = "plane"
				c.char = '^'

			} else {
				log.Printf("ffkln")
			}
		}

	}

	// Sweep Arm
	prevAngle := m.sweepAngle - 0.1
	for l := 0; l <= int(r); l++ {
		for interp := 0.0; interp <= 1.0; interp += 0.2 {
			theta := prevAngle + interp*(m.sweepAngle-prevAngle)
			x := cx + int(float64(l)*math.Cos(theta))
			y := cy + int(float64(l)*math.Sin(theta)*m.aspectRatio)

			if inBounds(m.width, m.height, x, y) {
				c := &m.buffer[y][x]
				c.kind = "sweep"
				c.sweepAge = 0
				c.char = ' '

			}
		}

	}

	for theta := 0.0; theta < 2*math.Pi; theta += 0.001 {
		x := cx + int(r*math.Cos(theta))
		y := cy + int(r*math.Sin(theta)*m.aspectRatio)
		if inBounds(m.width, m.height, x, y) {
			c := &m.buffer[y][x]
			c.char = getOutlineChar(theta)
			c.kind = "ring"

		}

	}

	tickMarks := []float64{0, 90, 180, 270}
	tickLabels := [][]rune{
		{'0'},
		{'9', '0'},
		{'1', '8', '0'},
		{'2', '7', '0'},
	}
	for i, tick := range tickMarks {
		theta := tick * math.Pi / 180

		x := cx + int((r+3)*math.Cos(theta+float64(m.northOffset)))
		y := cy + int((r+3)*math.Sin(theta+float64(m.northOffset))*m.aspectRatio)
		for j, r := range tickLabels[i] {
			if inBounds(m.width, m.height, x+j, y) {
				c := &m.buffer[y][x+j]
				c.char = r
				c.kind = "label"

			}

		}

	}

	var b strings.Builder
	for _, row := range m.buffer {

		for _, c := range row {
			if c.kind == "ring" {
				b.WriteString(frameBg.Render(" "))
				continue

			}

			if c.kind == "plane" {
				switch {
				case c.sweepAge <= 10:
					b.WriteString(brightGreen.Render(string(c.char)))

				case c.sweepAge > 10 && c.sweepAge <= 25:
					b.WriteString(mediumGreen.Render(string(c.char)))

				case c.sweepAge > 25 && c.sweepAge <= 50:
					b.WriteString(dimGreen.Render(string(c.char)))

				default:
					b.WriteString(string(c.char))
				}
				continue
			}

			switch {
			case c.sweepAge == 0:
				b.WriteString(brightGreenBg.Render(string(c.char)))

			case c.sweepAge > 0 && c.sweepAge <= 3:
				b.WriteString(mediumGreenBg.Render(string(c.char)))

			case c.sweepAge > 3 && c.sweepAge <= 12:
				b.WriteString(dimGreenBg.Render(string(c.char)))

			default:
				b.WriteString(string(c.char))
			}

		}

		b.WriteByte('\n')
	}

	return b.String()
}

func main() {
	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		log.Fatalf("err: %w", err)
	}

	defer f.Close()

	p := tea.NewProgram(model{
		radarRange:  75,
		aspectRatio: 0.5,
		lat:         0,
		lon:         0,
	}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}

}

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

var brightGreen = lipgloss.Color("#00ff00")
var mediumGreen = lipgloss.Color("#00bc00")
var dimGreen = lipgloss.Color("#007900")

var brightGreenBg = lipgloss.NewStyle().Background(lipgloss.Color("#00ff00"))
var mediumGreenBg = lipgloss.NewStyle().Background(lipgloss.Color("#009900"))
var dimGreenBg = lipgloss.NewStyle().Background(lipgloss.Color("#003300"))
var emptyBg = lipgloss.NewStyle().Background(lipgloss.Color("#1b1c1c"))
var frameBg = lipgloss.NewStyle().Background(lipgloss.Color("#3b3a3a"))

type plane struct {
	hex                  string
	lat                  float64
	lon                  float64
	bearingFromObserver  float64
	distanceFromObserver float64
}

type model struct {
	initialPlanesLoaded bool
	width               int
	height              int
	maxR                float64
	aspectRatio         float64
	sweepAngle          float64
	northOffset         float64
	radarRange          int
	buffer              [][]cell
	planes              []plane
	lat                 float64
	lon                 float64
}

type cell struct {
	char     rune
	kind     string
	sweepAge int
}

type tickMsg time.Time

func doTick() tea.Cmd {
	return tea.Tick(time.Millisecond*200, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func inBounds(width int, height int, x int, y int) bool {
	if x > 0 && x < width && y > 0 && y < height {
		return true
	}
	return false
}

func withinSweep(bearing, sweepAngle, width, northOffset float64) bool {
	bearing = math.Mod(bearing+northOffset, 2*math.Pi)
	sweepAngle = math.Mod(sweepAngle, 2*math.Pi)

	diff := math.Mod(sweepAngle-bearing+2*math.Pi, 2*math.Pi)

	return diff <= width
}

func (m model) Init() tea.Cmd {
	return doTick()
}

func (m model) GetPlanes() []plane {
	planes := []plane{
		{lat: 41.0, lon: -73.0, hex: "AAA123"}, // northeast ~40NM
		{lat: 40.3, lon: -74.5, hex: "BBB123"}, // southwest ~40NM
		{lat: 40.7, lon: -73.3, hex: "CCC123"}, // east ~40NM
		{lat: 41.2, lon: -74.0, hex: "DDD123"}, // north ~40NM
		{lat: 40.2, lon: -73.8, hex: "EEE123"}, // southeast ~40NM
	}

	for i, _ := range planes {
		m.SetPlaneLocationDetails(&planes[i])
	}
	return planes

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

		if !m.initialPlanesLoaded {
			m.planes = m.GetPlanes()
			m.initialPlanesLoaded = true
		}
		return m, nil

	case tickMsg:
		m.sweepAngle += 0.1
		if m.sweepAngle >= 2*math.Pi {
			m.sweepAngle = 0
			m.planes = m.GetPlanes()
		}

		for y := range m.buffer {
			for x := range m.buffer[y] {
				c := &m.buffer[y][x]
				if c.sweepAge < 100 {
					c.sweepAge = c.sweepAge + 1
				}

			}
		}

		return m, doTick()
	}

	return m, nil
}

func (m model) SetPlaneLocationDetails(p *plane) {
	curr_location := haversine.Coord{Lat: m.lat, Lon: m.lon}
	planeLocation := haversine.Coord{Lat: p.lat, Lon: p.lon}

	mi, _ := haversine.Distance(curr_location, planeLocation)
	nm := mi / 1.15078
	scale := float64(m.maxR-4) / float64(m.radarRange)
	virtualDistance := min(nm, float64(m.radarRange)) * scale

	p.distanceFromObserver = virtualDistance

	lat0Rad := m.lat
	lat1Rad := p.lat
	dLonRad := p.lon - m.lon

	y := math.Sin(dLonRad) * math.Cos(lat1Rad)
	x := math.Cos(lat0Rad)*math.Sin(lat1Rad) - math.Sin(lat0Rad)*math.Cos(lat1Rad)*math.Cos(dLonRad)
	bearing := math.Atan2(y, x)
	if bearing < 0 {
		bearing += 2 * math.Pi
	}
	p.bearingFromObserver = bearing
	log.Printf("SetPlane: lat=%.4f, lon=%.4f → bearing=%.4f, dist=%.4f",
		p.lat, p.lon, bearing, virtualDistance)
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	radar := m.renderRadar()
	return radar

}

func (m model) renderRadar() string {
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
			if c.kind != "plane" {
				c.char = ' '
				c.kind = "blank"
			}

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

	// Sweep Arm
	prevAngle := m.sweepAngle - 0.1
	for l := 0; l <= int(r); l++ {
		for interp := 0.0; interp <= 1.0; interp += 0.2 {
			theta := prevAngle + interp*(m.sweepAngle-prevAngle)
			x := cx + int(float64(l)*math.Sin(theta))
			y := cy - int(float64(l)*math.Cos(theta)*m.aspectRatio)

			if inBounds(m.width, m.height, x, y) {
				c := &m.buffer[y][x]
				c.kind = "sweep"
				c.sweepAge = 0
				c.char = ' '
			}
		}
	}

	for phi := 0.0; phi < 2*math.Pi; phi += 0.001 {
		x := cx + int(r*math.Sin(phi))
		y := cy - int(r*math.Cos(phi)*m.aspectRatio)
		if inBounds(m.width, m.height, x, y) {
			c := &m.buffer[y][x]
			c.char = ' '
			c.kind = "ring"
		}
	}

	for i, p := range m.planes {
		if withinSweep(p.bearingFromObserver, m.sweepAngle, 0.5, m.northOffset) {
			displayBearing := p.bearingFromObserver + m.northOffset
			posX := cx + int(p.distanceFromObserver*math.Sin(displayBearing)+m.northOffset)
			posY := cy - int(p.distanceFromObserver*math.Cos(displayBearing)*m.aspectRatio)

			log.Printf("Plane %d: bearing=%.4f, sweepAngle=%.4f, diff=%.4f, withinSweep=%v",
				i, p.bearingFromObserver, m.sweepAngle,
				math.Abs(math.Mod(p.bearingFromObserver-m.sweepAngle, 2*math.Pi)),
				withinSweep(p.bearingFromObserver, m.sweepAngle, 0.5, m.northOffset))
			log.Printf("  displayBearing=%.4f, sin=%.4f, cos=%.4f, dist=%.4f, posX=%d, posY=%d",
				displayBearing, math.Sin(displayBearing), math.Cos(displayBearing),
				p.distanceFromObserver, posX, posY)

			log.Printf("  trying to draw at posX=%d posY=%d inBounds=%v", posX, posY, inBounds(m.width, m.height, posX, posY))

			if inBounds(m.width, m.height, posX, posY) {
				c := &m.buffer[posY][posX]
				c.kind = "plane"
				c.char = '^'
			}
		} else {
			log.Printf("Plane %d NOT in sweep: bearing=%.4f, sweepAngle=%.4f",
				i, p.bearingFromObserver, m.sweepAngle)
		}
	}

	for phi := 0.0; phi < 2*math.Pi; phi += 0.001 {
		x := cx + int(r*math.Sin(phi))
		y := cy - int(r*math.Cos(phi)*m.aspectRatio)
		if inBounds(m.width, m.height, x, y) {
			c := &m.buffer[y][x]
			c.char = ' '
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
		phi := tick * math.Pi / 180
		phi += m.northOffset

		x := cx + int((r+3)*math.Sin(phi))
		y := cy - int((r+3)*math.Cos(phi)*m.aspectRatio)
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
			style := lipgloss.NewStyle()

			switch {
			case c.sweepAge == 0:
				style = style.Background(brightGreen)

			case c.sweepAge > 0 && c.sweepAge <= 3:
				style = style.Background(mediumGreen)

			case c.sweepAge > 3 && c.sweepAge <= 12:
				style = style.Background(dimGreen)

			}

			if c.kind == "plane" {

				switch {
				case c.sweepAge <= 10:
					style = style.Foreground(brightGreen)

				case c.sweepAge > 10 && c.sweepAge <= 30:
					style = style.Foreground(mediumGreen)

				case c.sweepAge > 30 && c.sweepAge <= 60:
					style = style.Foreground(dimGreen)

				case c.sweepAge == 99:
					c.kind = "blank"
					c.char = ' '
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

func main() {
	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		log.Fatalf("err: %w", err)
	}

	defer f.Close()

	p := tea.NewProgram(model{
		radarRange:          150,
		aspectRatio:         0.5,
		lat:                 40.7128,
		lon:                 -74.0060,
		initialPlanesLoaded: false,
	}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}

}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/umahmood/haversine"
)

var brightGreen = lipgloss.Color("#00ff00")
var mediumGreen = lipgloss.Color("#00bc00")
var dimGreen = lipgloss.Color("#007900")

var frameBg = lipgloss.NewStyle().Background(lipgloss.Color("#3b3a3a"))
var emptyBg = lipgloss.NewStyle().Background(lipgloss.Color("#1b1c1c"))

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

const (
	MIN_RADAR_RANGE      = 20
	MAX_RADAR_RANGE      = 200
	DEFAULT_RADAR_RANGE  = 70
	DEFAULT_ASPECT_RATIO = 0.5
	DEFAULT_LAT          = 53.79538
	DEFAULT_LON          = -1.66134
	DEFAULT_NORTH_OFFSET = 0.0
)

type adsbResponse struct {
	Planes []plane `json:"ac"`
}

type plane struct {
	Hex                  string  `json:"hex"`
	FlightCode           string  `json:"flight"`
	Lat                  float64 `json:"lat"`
	Lon                  float64 `json:"lon"`
	BearingFromObserver  float64
	DistanceFromObserver float64
}

type model struct {
	initialPlanesLoaded bool
	width               int
	height              int
	aspectRatio         float64
	sweepAngle          float64
	northOffset         float64
	radarRange          int
	buffer              [][]cell
	planes              []plane
	visiblePlanes       map[string]bool
	lat                 float64
	lon                 float64
	showModal           bool
	tbl                 table.Model
	tableLoaded         bool
	latInput            textinput.Model
	lonInput            textinput.Model
	modalFocused        bool
	getLiveFlights      bool
}

type cell struct {
	char     rune
	kind     string
	sweepAge int
}

type tickMsg time.Time

func GetLocalFlights(lat float64, lon float64, radius float64) []plane {
	url := fmt.Sprintf("https://api.adsb.lol/v2/point/%.4f/%.4f/%f", lat, lon, radius)

	var adsbResponse adsbResponse
	res, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("API response: %s", string(bodyBytes))

	if err := json.Unmarshal(bodyBytes, &adsbResponse); err != nil {
		log.Fatal(err)
	}

	log.Printf("adsbResponse: %+v", adsbResponse.Planes)

	return adsbResponse.Planes
}

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
	bearing = math.Mod(bearing-northOffset, 2*math.Pi)
	sweepAngle = math.Mod(sweepAngle, 2*math.Pi)

	diff := math.Mod(sweepAngle-bearing+2*math.Pi, 2*math.Pi)

	return diff <= width
}

func (m model) Init() tea.Cmd {
	return doTick()
}

func randJitter() float64 {
	return (rand.Float64())
}

func (m *model) GetPlanes() []plane {
	if m.getLiveFlights {

		planes := GetLocalFlights(m.lat, m.lon, float64(m.radarRange))
		for i := range planes {
			m.SetPlaneLocationDetails(&planes[i])
		}
		return planes
	}

	rand.Seed(time.Now().UnixNano())
	baseLat := m.lat
	baseLon := m.lon

	// Calculate positions for planes 30nm north and 30nm east
	// 30nm north: approximately 0.5 degrees latitude north
	// 30nm east: approximately 0.5 degrees longitude east (at this latitude)
	planes := []plane{
		{Lat: baseLat + 0.5, Lon: baseLon, Hex: "NORTH001", FlightCode: "NORTH30"},
		{Lat: baseLat, Lon: baseLon + 0.5, Hex: "EAST001", FlightCode: "EAST30"},
	}

	for i := range planes {
		m.SetPlaneLocationDetails(&planes[i])
	}
	return planes
}

func (m *model) UpdatePlaneRow(p plane) tea.Cmd {
	log.Printf("updating row for %s", p.Hex)

	rows := m.tbl.Rows()

	index := -1
	for i := range rows {
		if rows[i][0] == p.Hex {
			index = i
		}
	}

	newRow := table.Row{
		p.Hex,
		p.FlightCode,
		fmt.Sprintf("%.4f", p.Lat),
		fmt.Sprintf("%.4f", p.Lon),
		fmt.Sprintf("%.4f", p.DistanceFromObserver),
	}

	var newRows []table.Row
	if index == -1 {
		newRows = append([]table.Row{newRow}, rows...)
	} else {
		newRows = append([]table.Row{newRow}, append(rows[:index], rows[index+1:]...)...)
	}
	log.Printf("table rows : %+v", newRows)

	m.tbl.SetRows(newRows)

	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Always update table
	m.tbl, cmd = m.tbl.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle text input updates when modal is focused
		if m.showModal && m.modalFocused {
			switch msg.String() {
			case "tab":
				// Switch focus between inputs
				if m.latInput.Focused() {
					m.latInput.Blur()
					m.lonInput.Focus()
				} else {
					m.lonInput.Blur()
					m.latInput.Focus()
				}
				return m, tea.Batch(cmds...)
			case "enter":
				// Apply the new coordinates
				if latStr := m.latInput.Value(); latStr != "" {
					if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
						m.lat = lat
					}
				}
				if lonStr := m.lonInput.Value(); lonStr != "" {
					if lon, err := strconv.ParseFloat(lonStr, 64); err == nil {
						m.lon = lon
					}
				}
				m.showModal = false
				m.modalFocused = false
				m.latInput.Blur()
				m.lonInput.Blur()
				return m, tea.Batch(cmds...)
			case "esc":
				// Cancel and close modal
				m.showModal = false
				m.modalFocused = false
				m.latInput.Blur()
				m.lonInput.Blur()
				// Reset inputs to current values
				m.latInput.SetValue(fmt.Sprintf("%.4f", m.lat))
				m.lonInput.SetValue(fmt.Sprintf("%.4f", m.lon))
				return m, tea.Batch(cmds...)
			}

			// Update the focused input
			if m.latInput.Focused() {
				m.latInput, cmd = m.latInput.Update(msg)
				cmds = append(cmds, cmd)
			} else if m.lonInput.Focused() {
				m.lonInput, cmd = m.lonInput.Update(msg)
				cmds = append(cmds, cmd)
			}

			return m, tea.Batch(cmds...)
		}

		// Handle regular key messages when modal is not focused
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "]":
			m.northOffset += 0.1
			return m, tea.Batch(cmds...)

		case "[":
			m.northOffset -= 0.1
			return m, tea.Batch(cmds...)

		case "=":
			if m.radarRange < MAX_RADAR_RANGE {
				m.radarRange += 10
			}
			return m, tea.Batch(cmds...)

		case "-":
			if m.radarRange > MIN_RADAR_RANGE {
				m.radarRange -= 10
			}
			return m, tea.Batch(cmds...)

		case "m":
			m.showModal = !m.showModal
			if m.showModal {
				m.modalFocused = true
				m.latInput.Focus()
				m.latInput.SetValue(fmt.Sprintf("%.4f", m.lat))
				m.lonInput.SetValue(fmt.Sprintf("%.4f", m.lon))
			} else {
				m.modalFocused = false
				m.latInput.Blur()
				m.lonInput.Blur()
			}
			return m, tea.Batch(cmds...)
		}
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.buffer = make([][]cell, m.height)
		for y := range m.buffer {
			m.buffer[y] = make([]cell, m.width/2)
			for x := range m.buffer[y] {
				m.buffer[y][x] = cell{' ', "blank", int(100)}
			}
		}

		if !m.tableLoaded {
			columns := []table.Column{
				{Title: "ID", Width: 6},
				{Title: "FLT", Width: 8},
				{Title: "LAT", Width: 8},
				{Title: "LON", Width: 8},
				{Title: "DIST(NM)", Width: 8},
			}
			rows := []table.Row{}

			tableHeight := m.height/2 - 2
			if tableHeight < 5 {
				tableHeight = 5
			}

			m.tbl = table.New(
				table.WithColumns(columns),
				table.WithRows(rows),
				table.WithFocused(true),
				table.WithHeight(tableHeight),
			)

			// Set default styles with basic customization
			s := table.DefaultStyles()
			s.Header = s.Header.
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240")).
				BorderBottom(true).
				Bold(false)
			s.Selected = s.Selected.
				Foreground(lipgloss.Color("black")).
				Background(mediumGreen).
				Bold(false)
			m.tbl.SetStyles(s)

			m.tableLoaded = true
		}
		if !m.initialPlanesLoaded {
			m.planes = m.GetPlanes()
			m.initialPlanesLoaded = true
		}
		return m, tea.Batch(cmds...)

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
		currentVisible := make(map[string]bool)
		currentInSweep := make(map[string]bool)
		for _, p := range m.planes {
			// Track all planes within range, regardless of sweep position
			if p.DistanceFromObserver <= float64(m.radarRange) {
				currentVisible[p.Hex] = true

				// Track planes currently in sweep
				if withinSweep(p.BearingFromObserver, m.sweepAngle, 0.5, m.northOffset) {
					currentInSweep[p.Hex] = true
					if !m.visiblePlanes[p.Hex] {
						m.UpdatePlaneRow(p)
						cmds = append(cmds, tea.Printf(""))
					}
				}
			}
		}

		// Remove planes from table that are no longer visible
		rows := m.tbl.Rows()
		var newRows []table.Row
		for _, row := range rows {
			planeID := row[0]
			if currentVisible[planeID] {
				newRows = append(newRows, row)
			}
		}
		m.tbl.SetRows(newRows)

		m.visiblePlanes = currentInSweep
		cmds = append(cmds, doTick())

		return m, tea.Batch(cmds...)
	}

	return m, tea.Batch(cmds...)
}

func (m model) SetPlaneLocationDetails(p *plane) {
	curr_location := haversine.Coord{Lat: m.lat, Lon: m.lon}
	planeLocation := haversine.Coord{Lat: p.Lat, Lon: p.Lon}

	mi, _ := haversine.Distance(curr_location, planeLocation)
	nm := mi / 1.15078

	p.DistanceFromObserver = nm

	lat0Rad := m.lat * math.Pi / 180
	lat1Rad := p.Lat * math.Pi / 180
	dLonRad := (p.Lon - m.lon) * math.Pi / 180

	y := math.Sin(dLonRad) * math.Cos(lat1Rad)
	x := math.Cos(lat0Rad)*math.Sin(lat1Rad) - math.Sin(lat0Rad)*math.Cos(lat1Rad)*math.Cos(dLonRad)
	bearing := math.Atan2(y, x)
	if bearing < 0 {
		bearing += 2 * math.Pi
	}
	p.BearingFromObserver = bearing
	log.Printf("SetPlane: lat=%.4f, lon=%.4f → bearing=%.4f, dist=%.4f",
		p.Lat, p.Lon, bearing, nm)
}

func (m *model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	bearingDegrees := m.northOffset * 180 / math.Pi
	if bearingDegrees < 0 {
		bearingDegrees += 360
	}

	statusBar := lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Height(1).
		Width(m.width).
		Render(fmt.Sprintf("Range: %d NM  -\\= |  Bearing: %.0f° [\\] |  lat: %f   lon: %f  m to change", m.radarRange, bearingDegrees, m.lat, m.lon))

	radar := m.renderRadar(m.width/2, m.height)
	tableStr := lipgloss.NewStyle().
		Height(m.height).
		Width(m.width / 2).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center).
		Render(baseStyle.Render(m.tbl.View()))

	main := lipgloss.JoinVertical(
		lipgloss.Left,

		lipgloss.JoinHorizontal(
			lipgloss.Top,
			radar,
			tableStr,
		),
		statusBar,
	)

	if m.showModal {
		// Create modal content with text inputs
		modalContent := lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Render("Set Observer Location"),
			"",
			"Latitude:",
			m.latInput.View(),
			"",
			"Longitude:",
			m.lonInput.View(),
			"",
			lipgloss.NewStyle().Faint(true).Render("Tab: Switch | Enter: Apply | Esc: Cancel"),
		)

		// Overlay modal on top of main UI
		overlayModal := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			Padding(1, 2).
			Background(lipgloss.Color("#222")).
			Foreground(lipgloss.Color("#fff")).
			Align(lipgloss.Center, lipgloss.Center).
			Width(50).
			Height(12).
			Render(modalContent)

		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			overlayModal,
		)
	}

	return main

}

func (m model) renderRadar(width, height int) string {
	maxRx := float64(width/2) - 5
	maxRy := float64(height)/(2.0*m.aspectRatio) - 5
	maxR := min(maxRx, maxRy)
	r := maxR

	cx := width / 2
	cy := height / 2

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
			if inBounds(width, height, x+i, y) {
				c := &m.buffer[y][x+i]
				c.kind = "label"
				c.char = r
			}
		}
	}

	// Sweep Arm
	prevAngle := m.sweepAngle - 0.15
	for l := 0; l <= int(r); l++ {
		for interp := 0.0; interp <= 1.0; interp += 0.1 {
			theta := prevAngle + interp*(m.sweepAngle-prevAngle)
			x := cx + int(float64(l)*math.Sin(theta))
			y := cy - int(float64(l)*math.Cos(theta)*m.aspectRatio)

			if inBounds(width, height, x, y) {
				c := &m.buffer[y][x]
				c.kind = "sweep"
				c.sweepAge = int(interp * 10)
				c.char = ' '
			}
		}
	}

	for phi := 0.0; phi < 2*math.Pi; phi += 0.001 {
		x := cx + int(r*math.Sin(phi))
		y := cy - int(r*math.Cos(phi)*m.aspectRatio)
		if inBounds(width, height, x, y) {
			c := &m.buffer[y][x]
			c.char = ' '
			c.kind = "ring"
		}
	}

	for _, p := range m.planes {
		if withinSweep(p.BearingFromObserver, m.sweepAngle, 0.5, m.northOffset) {
			if p.DistanceFromObserver > float64(m.radarRange) {
				continue
			}

			scale := float64(maxR-4) / float64(m.radarRange)
			virtualDistance := p.DistanceFromObserver * scale

			displayBearing := p.BearingFromObserver - m.northOffset
			posX := cx + int(virtualDistance*math.Sin(displayBearing))
			posY := cy - int(virtualDistance*math.Cos(displayBearing)*m.aspectRatio)

			dx := float64(posX - cx)
			dy := float64(posY - cy)

			if inBounds(width, height, posX, posY) && math.Sqrt(dx*dx+dy*dy) < r {
				c := &m.buffer[posY][posX]
				c.kind = "plane"
				c.char = '^'

				c.sweepAge = 0
			}
		}
	}

	for phi := 0.0; phi < 2*math.Pi; phi += 0.001 {
		x := cx + int(r*math.Sin(phi))
		y := cy - int(r*math.Cos(phi)*m.aspectRatio)
		if inBounds(width, height, x, y) {
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
		phi -= m.northOffset

		x := cx + int((r+3)*math.Sin(phi))
		y := cy - int((r+3)*math.Cos(phi)*m.aspectRatio)
		for j, r := range tickLabels[i] {
			if inBounds(width, height, x+j, y) {
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
			case c.sweepAge <= 2:
				style = style.Background(brightGreen)

			case c.sweepAge > 2 && c.sweepAge <= 7:
				style = style.Background(mediumGreen)

			case c.sweepAge > 3 && c.sweepAge <= 12:
				style = style.Background(dimGreen)

			}

			if c.kind == "plane" {

				switch {
				case c.sweepAge <= 15:
					style = style.Foreground(brightGreen)

				case c.sweepAge > 15 && c.sweepAge <= 30:
					style = style.Foreground(mediumGreen)

				case c.sweepAge > 30 && c.sweepAge <= 60:
					style = style.Foreground(dimGreen)

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

func newModel() *model {
	latInput := textinput.New()
	latInput.Placeholder = "40.7128"
	latInput.CharLimit = 10
	latInput.Width = 15

	lonInput := textinput.New()
	lonInput.Placeholder = "-74.0060"
	lonInput.CharLimit = 11
	lonInput.Width = 15

	return &model{
		radarRange:          DEFAULT_RADAR_RANGE,
		aspectRatio:         DEFAULT_ASPECT_RATIO,
		lat:                 DEFAULT_LAT,
		lon:                 DEFAULT_LON,
		initialPlanesLoaded: false,
		tableLoaded:         false,
		visiblePlanes:       make(map[string]bool),
		showModal:           false,
		latInput:            latInput,
		lonInput:            lonInput,
		modalFocused:        false,
		getLiveFlights:      false,
	}
}

func main() {
	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		log.Fatalf("err: %w", err)
	}

	defer f.Close()

	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

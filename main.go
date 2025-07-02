package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"

	"github.com/muesli/termenv"
	"github.com/umahmood/haversine"
	gossh "golang.org/x/crypto/ssh"
)

var brightGreen = lipgloss.Color("#00ff00")
var mediumGreen = lipgloss.Color("#00bc00")
var dimGreen = lipgloss.Color("#007900")
var dimmestGreen = lipgloss.Color("#001b00")

var frameBg = lipgloss.NewStyle().Background(lipgloss.Color("#3b3a3a"))

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

const (
	MIN_RADAR_RANGE      = 1
	MAX_RADAR_RANGE      = 200
	DEFAULT_RADAR_RANGE  = 15
	DEFAULT_ASPECT_RATIO = 0.5
	DEFAULT_LAT          = 53.79538
	DEFAULT_LON          = -1.66134
	DEFAULT_NORTH_OFFSET = 0.0
)

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

	lat            float64
	lon            float64
	showModal      bool
	tbl            table.Model
	tableLoaded    bool
	latInput       textinput.Model
	lonInput       textinput.Model
	modalFocused   bool
	getLiveFlights bool
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

func withinSweep(bearing, sweepAngle, width, northOffset float64) bool {
	bearing = math.Mod(bearing-northOffset, 2*math.Pi)
	sweepAngle = math.Mod(sweepAngle, 2*math.Pi)

	diff := math.Mod(sweepAngle-bearing+2*math.Pi, 2*math.Pi)

	return diff <= width
}

func (m model) Init() tea.Cmd {
	return doTick()
}

func (m *model) GetPlanes() []plane {
	if m.getLiveFlights {
		planes := GetLocalFlights(m.lat, m.lon, float64(m.radarRange))
		for i := range planes {
			m.SetPlaneLocationDetails(&planes[i])
		}
		return planes
	}

	baseLat := m.lat
	baseLon := m.lon

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
	rows := m.tbl.Rows()

	index := -1
	for i := range rows {
		if rows[i][0] == p.FlightCode {
			index = i
		}
	}

	newRow := table.Row{
		p.FlightCode,
		p.RouteInfo.Airline,
		p.RouteInfo.OriginMunicipality,
		p.RouteInfo.DestMunicipality,
		fmt.Sprintf("%.2f", p.DistanceFromObserver),
	}

	var newRows []table.Row
	if index == -1 {
		newRows = append([]table.Row{newRow}, rows...)
	} else {
		newRows = append([]table.Row{newRow}, append(rows[:index], rows[index+1:]...)...)
	}

	m.tbl.SetRows(newRows)

	return nil
}

func (m *model) handleModalInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m, nil
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
		return m, nil
	case "esc":
		// Cancel and close modal
		m.showModal = false
		m.modalFocused = false
		m.latInput.Blur()
		m.lonInput.Blur()
		// Reset inputs to current values
		m.latInput.SetValue(fmt.Sprintf("%.4f", m.lat))
		m.lonInput.SetValue(fmt.Sprintf("%.4f", m.lon))
		return m, nil
	}

	// Update the focused input
	var cmd tea.Cmd
	if m.latInput.Focused() {
		m.latInput, cmd = m.latInput.Update(msg)
	} else if m.lonInput.Focused() {
		m.lonInput, cmd = m.lonInput.Update(msg)
	}
	return m, cmd
}

func (m *model) handleKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "]":
		m.northOffset += 0.1
		return m, nil
	case "[":
		m.northOffset -= 0.1
		return m, nil
	case "=":
		if m.radarRange < MAX_RADAR_RANGE {
			if m.radarRange == 1 {
				m.radarRange = 5
			} else {
				m.radarRange += 5
			}
		}
		return m, nil
	case "-":
		if m.radarRange > MIN_RADAR_RANGE {
			if m.radarRange == 5 {
				m.radarRange = 1
			} else {
				m.radarRange -= 5
			}
		}
		return m, nil
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
		return m, nil
	}
	return m, nil
}

func (m *model) handleWindowResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
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
			{Title: "FLT", Width: 8},
			{Title: "AIRLINE", Width: 16},
			{Title: "ORIGIN", Width: 18},
			{Title: "DEST", Width: 18},
			{Title: "DIST(NM)", Width: 10},
		}
		rows := []table.Row{}

		tableHeight := m.height / 2
		if tableHeight < 5 {
			tableHeight = 5
		}

		tableWidth := 0
		for _, column := range columns {
			tableWidth += column.Width
		}
		tableWidth += 10

		m.tbl = table.New(
			table.WithColumns(columns),
			table.WithRows(rows),
			table.WithFocused(true),
			table.WithHeight(tableHeight),
			table.WithWidth(tableWidth),
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
	return m, nil
}

func (m *model) handleTickMsg() (tea.Model, tea.Cmd) {
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
			currentVisible[p.FlightCode] = true

			// Track planes currently in sweep
			if withinSweep(p.BearingFromObserver, m.sweepAngle, 0.5, m.northOffset) {
				currentInSweep[p.FlightCode] = true
				if !m.visiblePlanes[p.FlightCode] {
					m.UpdatePlaneRow(p)
				}
			}
		}
	}

	// Remove planes from table that are no longer visible
	rows := m.tbl.Rows()
	var newRows []table.Row
	for _, row := range rows {
		flightCode := row[0]
		if currentVisible[flightCode] {
			newRows = append(newRows, row)
		}
	}
	m.tbl.SetRows(newRows)

	m.visiblePlanes = currentInSweep
	return m, doTick()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Always update table
	m.tbl, cmd = m.tbl.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.showModal && m.modalFocused {
			return m.handleModalInput(msg)
		}
		return m.handleKeyInput(msg)
	case tea.WindowSizeMsg:
		return m.handleWindowResize(msg)
	case tickMsg:
		return m.handleTickMsg()
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
		getLiveFlights:      true,
	}
}

func main() {
	var host string
	var port string
	flag.StringVar(&host, "host", "", "Host to listen on (default: all interfaces)")
	flag.StringVar(&port, "port", "22", "Port to listen on (default: 22)")
	flag.Parse()

	os.Setenv("TERM", "xterm-256color")
	os.Setenv("COLORTERM", "truecolor")

	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath("/var/lib/mysshapp/.ssh/termui_ed25519"),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		}),
		wish.WithKeyboardInteractiveAuth(func(ctx ssh.Context, challenger gossh.KeyboardInteractiveChallenge) bool {
			return true
		}),
		wish.WithMiddleware(
			radarBubbleteaMiddleware(),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Print("Could not start server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Print("Starting SSH server", "host", host, "port", port)
	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Print("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Println("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() { cancel() }()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Print("Could not stop server", "error", err)
	}
}

func radarBubbleteaMiddleware() wish.Middleware {
	teaHandler := func(s ssh.Session) *tea.Program {
		log.Print("New SSH session started")

		pty, _, active := s.Pty()
		if !active {
			log.Print("no active terminal, skipping")
			return nil
		}

		m := newModel()
		m.width = pty.Window.Width
		m.height = pty.Window.Height

		p := tea.NewProgram(
			m,
			tea.WithAltScreen(),
			tea.WithInput(s),
			tea.WithOutput(s),
		)
		return p
	}
	return bubbletea.MiddlewareWithProgramHandler(teaHandler, termenv.TrueColor)
}

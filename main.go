package main

import (
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	width       int
	height      int
	sweepAngle  float64
	northOffset float64
	radarRange  int
}

type cell struct {
	char rune
	kind string
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
		return m, nil

	case tickMsg:
		m.sweepAngle += 0.1
		if m.sweepAngle >= 2*math.Pi {
			m.sweepAngle = 0
		}

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

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	aspectRatio := 0.5

	maxRx := float64(m.width / 4)
	maxRy := float64(m.height) / (2.0 * aspectRatio)
	MaxR := min(maxRx, maxRy)
	r := MaxR - 4
	cx := int(maxRx)
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

	buffer := make([][]cell, m.height)
	for y := range buffer {
		buffer[y] = make([]cell, m.width)
		for x := range buffer[y] {
			buffer[y][x] = cell{' ', "blank"}
		}
	}

	for d := labelStepNM; d < float64(m.radarRange); d += labelStepNM {
		radius := d * charsPerNM
		x := cx + int(radius*math.Sin(0))
		y := cy - int(radius*math.Cos(0)*aspectRatio)

		label := strconv.Itoa(int(d))
		for i, r := range []rune(label) {
			if inBounds(m.width, m.height, x+i, y) {
				buffer[y][x+i] = cell{r, "ring"}
			}
		}
	}

	for l := 0; l <= int(r); l++ {
		x := cx + int(float64(l)*math.Cos(m.sweepAngle))
		y := cy + int(float64(l)*math.Sin(m.sweepAngle)*aspectRatio)

		if inBounds(m.width, m.height, x, y) {
			buffer[y][x] = cell{'#', "sweep"}
		}

	}

	for theta := 0.0; theta < 2*math.Pi; theta += 0.001 {
		x := cx + int(r*math.Cos(theta))
		y := cy + int(r*math.Sin(theta)*aspectRatio)
		if inBounds(m.width, m.height, x, y) {
			buffer[y][x] = cell{getOutlineChar(theta), "ring"}
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
		y := cy + int((r+3)*math.Sin(theta+float64(m.northOffset))*aspectRatio)
		for j, r := range tickLabels[i] {
			if inBounds(m.width, m.height, x+j, y) {
				buffer[y][x+j] = cell{r, "ring"}
			}

		}

	}

	var b strings.Builder
	for _, row := range buffer {
		for _, c := range row {
			b.WriteRune(c.char)
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
		radarRange: 75,
	}, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}

}

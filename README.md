# WhatPlaneIsThat

## Motivation

I've recently found myself [with the window open a lot](https://www.metoffice.gov.uk/about-us/news-and-media/media-centre/weather-and-climate-news/2025/june-2025-provisional-statistics), often hearing planes flying overhead and wondering where they were going. Combined with my recent discovery of [Bubble Tea](https://github.com/charmbracelet/bubbletea) for building TUIs and [Wish](https://github.com/charmbracelet/wish) for making SSH apps (through [terminal.shop](https://terminal.shop)), 

## What is it?

  ![Made with VHS](https://vhs.charm.sh/vhs-6rltGALyCvZRi8BpLoMcFP.gif)  
**WhatPlaneIsThat** is a terminal-based radar app you can SSH into. It shows a live radar-style display of planes flying near your location, using real data. The radar visualization is a fun and intuitive way to see what's overhead, where it's headed, and get a sense of the sky above you—all from your terminal.


## Tech Stack
- [Go](https://golang.org/)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Wish](https://github.com/charmbracelet/wish)
- [Lipgloss](https://github.com/charmbracelet/lipgloss) for styling
- [adsb.lol](https://adsb.lol/) for finding local flights
- [adsbdb](https://www.adsbdb.com/) for finding route details for specific flights
## Usage

1. **Clone the repo:**
   ```sh
   git clone https://github.com/yourusername/WhatPlaneIsThat.git
   cd WhatPlaneIsThat
   ```
2. **Build and run the SSH server:**
   ```sh
   go run main.go
   ```
   By default, the server listens on all interfaces (host="") and port 22. You can override these with command-line flags:
   ```sh
   go run main.go --host=127.0.0.1 --port=2222
   ```
3. **SSH into your server:**
  
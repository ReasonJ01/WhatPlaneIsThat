package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type adsbResponse struct {
	Planes []plane `json:"ac"`
}

type flightRouteResponse struct {
	Response struct {
		FlightRoute struct {
			Airline struct {
				Name string `json:"name"`
			} `json:"airline"`
			Origin struct {
				Name         string `json:"name"`
				CountryName  string `json:"country_name"`
				Municipality string `json:"municipality"`
			} `json:"origin"`
			Destination struct {
				Name         string `json:"name"`
				CountryName  string `json:"country_name"`
				Municipality string `json:"municipality"`
			} `json:"destination"`
		} `json:"flightroute"`
	} `json:"response"`
}

type FlightRoute struct {
	Airline            string
	OriginAirport      string
	OriginCountry      string
	OriginMunicipality string
	DestAirport        string
	DestCountry        string
	DestMunicipality   string
}

type plane struct {
	Hex                  string  `json:"hex"`
	FlightCode           string  `json:"flight"`
	Lat                  float64 `json:"lat"`
	Lon                  float64 `json:"lon"`
	Heading              float64 `json:"true_heading"`
	BearingFromObserver  float64
	DistanceFromObserver float64
	RouteInfo            FlightRoute
}

type cachedFlightRoute struct {
	route    FlightRoute
	cachedAt time.Time
}

var routeInfoCache = make(map[string]cachedFlightRoute)

const routeInfoCacheTTL = 10 * time.Minute

func createEmptyFlightRoute() FlightRoute {
	return FlightRoute{
		Airline:            "",
		OriginAirport:      "",
		OriginCountry:      "",
		OriginMunicipality: "",
		DestAirport:        "",
		DestCountry:        "",
		DestMunicipality:   "",
	}
}

func SetFlightRouteInfo(p *plane) {
	if _, ok := routeInfoCache[p.FlightCode]; ok {
		if time.Since(routeInfoCache[p.FlightCode].cachedAt) <= routeInfoCacheTTL {
			p.RouteInfo = routeInfoCache[p.FlightCode].route
			return
		}
	}

	url := fmt.Sprintf("https://api.adsbdb.com/v0/callsign/%s", strings.TrimSpace(p.FlightCode))

	var flightRouteInfo flightRouteResponse

	res, err := http.Get(url)
	if err != nil {
		p.RouteInfo = createEmptyFlightRoute()
		return
	}

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		p.RouteInfo = createEmptyFlightRoute()
		return
	}

	log.Printf("Flight route API response for %s: %s", p.FlightCode, string(bodyBytes))

	// Handle unknown callsign response
	if strings.Contains(string(bodyBytes), "\"response\":\"unknown callsign\"") {
		p.RouteInfo = createEmptyFlightRoute()
		routeInfoCache[p.FlightCode] = cachedFlightRoute{
			route:    p.RouteInfo,
			cachedAt: time.Now(),
		}
		return
	}

	if err := json.Unmarshal(bodyBytes, &flightRouteInfo); err != nil {
		p.RouteInfo = createEmptyFlightRoute()
		return
	}

	fr := flightRouteInfo.Response.FlightRoute

	flightRoute := FlightRoute{
		Airline:            fr.Airline.Name,
		OriginAirport:      fr.Origin.Name,
		OriginCountry:      fr.Origin.CountryName,
		OriginMunicipality: fr.Origin.Municipality,
		DestAirport:        fr.Destination.Name,
		DestCountry:        fr.Destination.CountryName,
		DestMunicipality:   fr.Destination.Municipality,
	}

	p.RouteInfo = flightRoute
	routeInfoCache[p.FlightCode] = cachedFlightRoute{
		route:    flightRoute,
		cachedAt: time.Now(),
	}
}

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

	for i := range adsbResponse.Planes {
		SetFlightRouteInfo(&adsbResponse.Planes[i])
	}

	return adsbResponse.Planes
}

package main

type CZF struct {
	Revision      string                     `json:"revision"`
	CustomerName  string                     `json:"customerName"`
	CoverageZones map[string]CZFCoverageZone `json:"coverageZones"`
}

type CZFCoverageZone struct {
	Network     []string  `json:"network"`
	Network6    []string  `json:"network6"`
	Coordinates CZFLatLon `json:"coordinates"`
}

type CZFLatLon struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

package app

import "github.com/ONSdigital/dis-search-algorithm/testset/stream"

const (
	docNameCPI         = "cpi-latest"
	docNameGrowth      = "growth-dataset"
	testCPITitle       = "Consumer price inflation, UK"
	testCPIURI         = "/economy/cpi"
	testGrowthTitle    = "Growth"
	testGrowthURI      = "/economy/growth"
	testShortCPITitle  = "CPI"
	testShortCPIURI    = "/cpi"
	testShortGrowthURI = "/growth"
)

var (
	testDocuments = []stream.Item{
		{Name: docNameCPI, Body: []byte(`{"title":"CPI"}`)},
		{Name: docNameGrowth, Body: []byte(`{"title":"Growth"}`)},
	}
)

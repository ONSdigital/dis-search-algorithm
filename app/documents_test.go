package app

import "github.com/ONSdigital/dis-search-algorithm/testset/stream"

const (
	docNameCPI         = "cpi-latest"
	docNameGrowth      = "growth-dataset"
	testGrowthTitle    = "Growth"
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

package traffic

import "github.com/anthonyrego/construct/pkg/geojson"

var SignalDataset = geojson.DatasetConfig{
	Endpoint:    "https://data.cityofnewyork.us/resource/bryy-vqd9.json",
	GeomColumn:  "the_geom",
	ExtraSelect: "onstreetna,fromstreet",
	CachePrefix: "traffic_signals",
}

var CenterlineDataset = geojson.DatasetConfig{
	Endpoint:    "https://data.cityofnewyork.us/resource/dpb9-ubdh.json",
	GeomColumn:  "the_geom",
	ExtraSelect: "trafdir,stname_lab",
	CachePrefix: "centerline",
}

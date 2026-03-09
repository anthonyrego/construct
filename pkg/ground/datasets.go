package ground

import "github.com/anthonyrego/construct/pkg/geojson"

var RoadbedDataset = geojson.DatasetConfig{
	Endpoint:    "https://data.cityofnewyork.us/resource/i36f-5ih7.json",
	GeomColumn:  "the_geom",
	CachePrefix: "roadbed",
}

var SidewalkDataset = geojson.DatasetConfig{
	Endpoint:    "https://data.cityofnewyork.us/resource/52n9-sdep.json",
	GeomColumn:  "the_geom",
	CachePrefix: "sidewalk",
}

var ParkDataset = geojson.DatasetConfig{
	Endpoint:    "https://data.cityofnewyork.us/resource/enfh-gkve.json",
	GeomColumn:  "multipolygon",
	ExtraSelect: "signname,typecategory",
	CachePrefix: "park",
}

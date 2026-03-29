package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"

	"github.com/anthonyrego/construct/pkg/engine"
	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/pipeline"
	"github.com/anthonyrego/construct/pkg/settings"
)

func main() {
	doImport := flag.Bool("import", false, "Import .cache/ data into data/map/ and exit")
	useFetch := flag.Bool("fetch", false, "Use old fetch-from-API pipeline instead of map data")
	doPipeline := flag.Bool("pipeline", false, "Run asset pipeline")
	pipelineBBL := flag.String("bbl", "", "Building BBL to process (with -pipeline)")
	pipelineStage := flag.String("stage", "", "Specific stage to run (with -pipeline)")
	flag.Parse()

	minLat, minLon, maxLat, maxLon := 40.700, -74.020, 40.747, -73.970

	if *doImport {
		if err := mapdata.Import(mapDataDir, minLat, minLon, maxLat, maxLon); err != nil {
			fmt.Println("Import failed:", err)
			os.Exit(1)
		}
		return
	}

	if *doPipeline {
		if *pipelineBBL == "" {
			fmt.Println("Usage: construct -pipeline -bbl=BUILDING_BBL [-stage=STAGE]")
			os.Exit(1)
		}
		store, _ := mapdata.Load(mapDataDir)
		runner := pipeline.NewRunner("data/assets", store)
		fmt.Printf("Pipeline: processing BBL %s\n", *pipelineBBL)
		var err error
		if *pipelineStage != "" {
			err = runner.RunStage(*pipelineBBL, *pipelineStage)
		} else {
			err = runner.RunBuilding(*pipelineBBL)
		}
		if err != nil {
			fmt.Println("Pipeline error:", err)
			os.Exit(1)
		}
		fmt.Println("Pipeline complete.")
		return
	}

	// Try to load system SDL3, fall back to embedded
	err := sdl.LoadLibrary(sdl.Path())
	if err != nil {
		fmt.Println("Loading embedded SDL3 library...")
		defer binsdl.Load().Unload()
	}

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		panic("failed to initialize SDL: " + err.Error())
	}
	defer sdl.Quit()

	fmt.Println("SDL Version:", sdl.GetVersion())

	ds := settings.Load("settings.json")

	e, err := engine.New("Construct - City Block", ds)
	if err != nil {
		panic(err)
	}
	defer e.Destroy()

	fmt.Println("GPU Driver:", e.Win.Device().Driver())

	game := &NYCGame{
		useFetch:   *useFetch,
		mapDataDir: mapDataDir,
	}

	if err := e.Run(game); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("\nGoodbye!")
}

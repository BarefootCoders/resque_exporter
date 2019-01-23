package main

import (
	"os"

	"github.com/BarefootCoders/resque_exporter"
)

func main() {
	resqueExporter.Run(os.Args[1:])
}

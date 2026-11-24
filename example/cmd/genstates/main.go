package main

import (
	"flag"
	"log"
	"os"

	worker "github.com/pantacor/lawgate/example"
	"github.com/pantacor/lawgate/gen"
)

func main() {
	check := flag.Bool("check", false, "report stale generated state types without writing")
	flag.Parse()
	if *check {
		checkFreshness()
		return
	}
	target := worker.StateTypeTarget()
	source, err := gen.Go(target.Machine, target.Options)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(target.Path, source, 0o644); err != nil {
		log.Fatal(err)
	}
}

func checkFreshness() {
	if err := worker.CheckWorkerStateTypes(os.Stdout); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	worker "github.com/pantacor/lawgate/example"
	"github.com/pantacor/lawgate/runner"
)

func main() {
	runner.Main([]runner.Entry{worker.Entry()})
}

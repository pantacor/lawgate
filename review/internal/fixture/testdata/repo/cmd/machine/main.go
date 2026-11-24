package main

import (
	shop "example.com/shop"
	"github.com/pantacor/lawgate/runner"
)

func main() {
	runner.Main([]runner.Entry{shop.Entry()})
}

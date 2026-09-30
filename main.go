package main

import (
	"flag"

	"github.com/renzoarr/mustclose/mustclose"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	cfg := mustclose.Config{}
	flag.BoolVar(&cfg.Names, "names", false, "include source names in diagnostics (requires SSA debug info; slower)")
	flag.Parse()

	singlechecker.Main(mustclose.NewAnalyzer(cfg))
}

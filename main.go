package main

import (
	"github.com/renzoarr/mustclose/mustclose"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(mustclose.NewAnalyzer())
}

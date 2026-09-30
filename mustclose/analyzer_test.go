package mustclose

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestDefault(t *testing.T) {
	cfg := Config{Names: false}
	analysistest.Run(t, analysistest.TestData(), NewAnalyzer(cfg), ".")
}

func TestDetailed(t *testing.T) {
	cfg := Config{Names: true}
	analysistest.Run(t, analysistest.TestData(), NewAnalyzer(cfg), ".")
}

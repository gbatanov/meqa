package main

import (
	"meqa/mqutil"
	"os"
	"path/filepath"
	"testing"
)

func TestMqgen(t *testing.T) {
	mqutil.Logger = mqutil.NewStdLogger()
	wd, _ := os.Getwd()
	meqaPath := filepath.Join(wd, "../../../testdata")
	swaggerPath := filepath.Join(meqaPath, "petstore_meqa.yml")
	algorithm := "all"
	verbose := false
	whitelistFile := ""
	run(&meqaPath, &swaggerPath, &algorithm, &verbose, &whitelistFile)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

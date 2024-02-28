package main

import (
	"crypto/tls"
	"flag"
	"fmt"

	"os"

	"path/filepath"

	"github.com/gbatanov/meqa/mqgen"
	"github.com/gbatanov/meqa/mqplan"
	"github.com/gbatanov/meqa/mqswag"
	"github.com/gbatanov/meqa/mqutil"
	"gopkg.in/resty.v1"
)

const VERSION = "v0.1.2"

const (
	meqaDataDir = "meqa_data"
	resultFile  = "result.yml"
	serverURL   = ""
)

func main() {
	genCommand := flag.NewFlagSet("gen", flag.ExitOnError)
	genCommand.SetOutput(os.Stdout)
	runCommand := flag.NewFlagSet("run", flag.ExitOnError)
	runCommand.SetOutput(os.Stdout)

	genMeqaPath := genCommand.String("d", meqaDataDir, "the directory where meqa config, log and output files reside")
	genSwaggerFile := genCommand.String("s", "", "the OpenAPI (Swagger) spec file path")
	algorithm := genCommand.String("a", "all", "the algorithm - simple, path, all")
	genVerbose := genCommand.Bool("v", false, "turn on verbose mode")

	runMeqaPath := runCommand.String("d", meqaDataDir, "the directory where meqa config, log and output files reside")
	runSwaggerFile := runCommand.String("s", "", "the meqa generated OpenAPI (Swagger) spec file path")
	testPlanFile := runCommand.String("p", "", "the test plan file name")
	resultPath := runCommand.String("r", "result.yml", "the test result file name (default result.yml in meqa_data dir)")
	testToRun := runCommand.String("t", "all", "the test to run")
	username := runCommand.String("u", "", "the username for basic HTTP authentication")
	password := runCommand.String("w", "", "the password for basic HTTP authentication")
	apitoken := runCommand.String("a", "", "the api token for bearer HTTP authentication")
	verbose := runCommand.Bool("v", false, "turn on verbose mode")

	flag.Usage = func() {
		fmt.Println("Usage: mqgo {gen|run} [options]")
		fmt.Println("generate: generate test plans to be used by run command")
		genCommand.PrintDefaults()

		fmt.Println("\nrun: run the tests the in a test plan file")
		runCommand.PrintDefaults()
	}

	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	var meqaPath *string
	var swaggerFile *string
	runType := ""
	switch os.Args[1] {
	case "gen":
		genCommand.Parse(os.Args[2:])
		meqaPath = genMeqaPath
		swaggerFile = genSwaggerFile
		runType = "gen"
	case "run":
		runCommand.Parse(os.Args[2:])
		meqaPath = runMeqaPath
		swaggerFile = runSwaggerFile
		runType = "run"
	default:
		flag.Usage()
		os.Exit(1)
	}
	if len(*swaggerFile) == 0 {
		fmt.Println("You must use -s option to provide a swagger/openapi yaml spec file. Use -h to see the options")
		os.Exit(1)
	}

	fi, err := os.Stat(*meqaPath)
	if os.IsNotExist(err) {
		fmt.Printf("Meqa directory %s doesn't exist.", *meqaPath)
		os.Exit(1)
	}
	if !fi.Mode().IsDir() {
		fmt.Printf("Meqa directory %s is not a directory.", *meqaPath)
		os.Exit(1)
	}

	if runType == "run" {
		if len(*resultPath) == 0 {
			rf := filepath.Join(*meqaPath, resultFile)
			resultPath = &rf
		}
	}

	mqutil.Logger = mqutil.NewFileLogger(filepath.Join(*meqaPath, "mqgo.log"))
	mqutil.Logger.Println(os.Args)

	if _, err := os.Stat(*swaggerFile); os.IsNotExist(err) {
		fmt.Printf("can't load swagger file at the following location %s", *swaggerFile)
		os.Exit(1)
	}

	if runType == "gen" && genCommand.Parsed() {
		err = mqgen.Run(meqaPath, swaggerFile, algorithm, genVerbose)

		if err != nil {
			fmt.Printf("got an err:\n%s", err.Error())
		}
		return
	} else if runType == "run" {
		runMeqa(meqaPath, swaggerFile, testPlanFile, resultPath, testToRun, username, password, apitoken, verbose)
	}

}

func runMeqa(meqaPath *string, swaggerFile *string, testPlanFile *string, resultPath *string,
	testToRun *string, username *string, password *string, apitoken *string, verbose *bool) {

	mqutil.Verbose = *verbose

	if len(*testPlanFile) == 0 {
		fmt.Println("You must use -p to specify a test plan file. Use -h to see more options.")
		return
	}

	if _, err := os.Stat(*testPlanFile); os.IsNotExist(err) {
		fmt.Printf("can't load test plan file at the following location %s", *testPlanFile)
		return
	}

	// load swagger.yml
	swagger, err := mqswag.CreateSwaggerFromURL(*swaggerFile, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}
	mqswag.ObjDB.Init(swagger)

	// load test plan
	mqplan.Current.Username = *username
	mqplan.Current.Password = *password
	mqplan.Current.ApiToken = *apitoken
	err = mqplan.Current.InitFromFile(*testPlanFile, &mqswag.ObjDB)
	if err != nil {
		mqutil.Logger.Printf("Error loading test plan: %s", err.Error())
	}

	// for testing, set the config to skip verifying https certificates
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	resty.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))

	mqplan.Current.ResultCounts = make(map[string]int)
	if *testToRun == "all" {
		for _, testSuite := range mqplan.Current.SuiteList {
			mqutil.Logger.Printf("\n---Test suite: %s\n", testSuite.Name)
			fmt.Printf("\n---Test suite: %s\n", testSuite.Name)
			counts, err := mqplan.Current.Run(testSuite.Name, nil)
			if err != nil {
				mqutil.Logger.Printf("err:\n%v", err)
			} else {
				mqutil.Logger.Println("success")
			}
			for k := range counts {
				mqplan.Current.ResultCounts[k] += counts[k]
			}
		}
	} else {
		mqutil.Logger.Printf("\n---\nTest suite: %s\n", *testToRun)
		fmt.Printf("\n---\nTest suite: %s\n", *testToRun)
		counts, err := mqplan.Current.Run(*testToRun, nil)
		if err != nil {
			mqutil.Logger.Printf("err:\n%v", err)
		} else {
			mqutil.Logger.Println("success")
		}
		for k := range counts {
			mqplan.Current.ResultCounts[k] += counts[k]
		}
	}
	mqplan.Current.LogErrors()
	mqplan.Current.PrintSummary()
	os.Remove(*resultPath)
	mqplan.Current.WriteResultToFile(*resultPath)
}

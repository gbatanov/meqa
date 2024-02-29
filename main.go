package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"

	"os"

	"path/filepath"

	"github.com/gbatanov/meqa/mqgen"
	"github.com/gbatanov/meqa/mqplan"
	"github.com/gbatanov/meqa/mqswag"
	"github.com/gbatanov/meqa/mqutil"
	"gopkg.in/resty.v1"
)

const VERSION = "v0.1.7"

const (
	HOST         = "192.168.76.95:8000"
	MeqaDataDir  = "meqa_data"
	ResultFile   = "result.yml"
	SwaggerFile  = "swagger.yaml"
	AuthorizeUrl = "log_in"
)

func main() {
	/*
		b, err := os.ReadFile("swagger.yaml")
		if err == nil {
			js, err := mqutil.YamlToJson(b)
			if err == nil {
				os.WriteFile("swagger_gen.json", js, fs.FileMode(os.O_RDWR))
			}
		}
		os.Exit(1)
	*/
	meqaPath := flag.String("d", MeqaDataDir, "the directory where meqa log and output files reside")
	swaggerFile := flag.String("s", SwaggerFile, "the OpenAPI (Swagger) spec file path")
	verbose := flag.Bool("v", false, "turn on verbose mode")
	//	resultPath := flag.String("r", ResultFile, "the test result file name")
	username := flag.String("u", "admin", "the username for basic HTTP authentication")
	password := flag.String("w", "admin", "the password for basic HTTP authentication")
	apitoken := flag.String("t", "", "the api token for bearer HTTP authentication")

	flag.Usage = func() {
		fmt.Println("Usage: mqgo [options]")
		flag.PrintDefaults()
	}

	flag.Parse()

	algorithm := "path"

	if len(*swaggerFile) == 0 {
		fmt.Println("You must use -s option to provide a swagger/openapi yaml spec file. Use -h to see the options")
		flag.Usage()
		os.Exit(1)
	}

	fi, err := os.Stat(*meqaPath)
	if os.IsNotExist(err) {
		fmt.Printf("Meqa directory %s doesn't exist.", *meqaPath)
		flag.Usage()
		os.Exit(1)
	}
	if !fi.Mode().IsDir() {
		fmt.Printf("Meqa directory %s is not a directory.", *meqaPath)
		os.Exit(1)
	}
	testSuiteFile := filepath.Join(*meqaPath, algorithm+".yml")
	rf := filepath.Join(*meqaPath, ResultFile)
	resultPath := &rf

	mqutil.Logger = mqutil.NewFileLogger(filepath.Join(*meqaPath, "mqgo.log"))
	mqutil.Logger.Println(os.Args)

	if _, err := os.Stat(*swaggerFile); os.IsNotExist(err) {
		fmt.Printf("can't load swagger file at the following location %s", *swaggerFile)
		flag.Usage()
		os.Exit(1)
	}

	mqutil.Verbose = *verbose

	err = mqgen.Run(meqaPath, swaggerFile, &algorithm)

	if err != nil {
		fmt.Printf("got an err:\n%s", err.Error())
		return
	}

	if _, err := os.Stat(testSuiteFile); os.IsNotExist(err) {
		fmt.Println("Test suite file is not generated. Use -h to see more options.")
		return
	}

	runTests(meqaPath, swaggerFile, &testSuiteFile, resultPath, username, password, apitoken)

}

// Запуск сгенерированных тестов
func runTests(dataPath *string, swaggerFile *string, testSuiteFile *string, resultPath *string,
	username *string, password *string, apitoken *string) {

	// load swagger.yml
	swagger, err := mqswag.CreateSwaggerFromFile(*swaggerFile, *dataPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}
	mqswag.ObjDB.Init(swagger)
	if len(swagger.Host) == 0 {
		swagger.Host = HOST
	}
	// load test plan
	err = mqplan.CurrentSuite.InitFromFile(*testSuiteFile, &mqswag.ObjDB)
	if err != nil {
		mqutil.Logger.Printf("Error loading test plan: %s", err.Error())
	}

	mqplan.CurrentSuite.Username = *username
	mqplan.CurrentSuite.Password = *password
	mqplan.CurrentSuite.ApiToken = *apitoken

	// for testing, set the config to skip verifying https certificates
	resty.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	resty.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))

	mqplan.CurrentSuite.ResultCounts = make(map[string]int)

	mqplan.CurrentSuite.StartUp()

	for _, testCase := range mqplan.CurrentSuite.TestList {
		mqutil.Logger.Printf("\n---Test case path: %s\n", testCase.Name)
		log.Println("========" + testCase.Name + "=============")

		if testCase.Name == "/configuration/set-serial" {
			continue
		}
		if testCase.Name == "/signin" {
			continue
		}
		if testCase.Name == "/signout" {
			continue
		}
		counts, err := mqplan.CurrentSuite.Run(testCase.Name, nil)

		if err != nil {
			mqutil.Logger.Printf("%s", err.Error())
		} else {
			mqutil.Logger.Println("=== test success ===")
		}
		for k := range counts {
			mqplan.CurrentSuite.ResultCounts[k] += counts[k]
		}
		mqutil.Logger.Println(" ")
	}

	// Выводим список ошибок в консоль
	if mqplan.CurrentSuite.ResultCounts["Failed"] < 10 {
		mqplan.CurrentSuite.LogErrors()
	}
	// Выводим суммарный итог по тесту
	mqplan.CurrentSuite.PrintSummary()
	// Пишем реузльтирующий файл
	// TODO: Переделать, самая бесполезная вещь, фактически дуюлирует path.yml
	os.Remove(*resultPath)
	mqplan.CurrentSuite.WriteResultToFile(*resultPath)
}

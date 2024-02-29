package mqgen

import (
	"fmt"

	"os"
	"path/filepath"

	"github.com/gbatanov/meqa/mqplan"
	"github.com/gbatanov/meqa/mqswag"
	"github.com/gbatanov/meqa/mqutil"
)

const (
	meqaDataDir = "meqa_data"
	algoSimple  = "simple"
	algoObject  = "object"
	algoPath    = "path"
	algoAll     = "all"
)

var algoList []string = []string{algoSimple, algoPath}

/*
	func main() {
		mqutil.Logger = mqutil.NewStdLogger()

		swaggerJSONFile := filepath.Join(meqaDataDir, "swagger.yml")
		meqaPath := flag.String("d", meqaDataDir, "the directory where we put the generated files")
		swaggerFile := flag.String("s", swaggerJSONFile, "the swagger.yml file location")
		algorithm := flag.String("a", "all", "the algorithm - simple, object, path, all")
		verbose := flag.Bool("v", false, "turn on verbose mode")
		whitelistFile := flag.String("w", "", "the whitelist.txt file location")

		flag.Parse()
		run(meqaPath, swaggerFile, algorithm, verbose, whitelistFile)
	}
*/
func Run(meqaPath *string, swaggerFile *string, algorithm *string, verbose *bool) error {
	mqutil.Verbose = *verbose

	swaggerJsonPath := *swaggerFile
	if fi, err := os.Stat(swaggerJsonPath); os.IsNotExist(err) || fi.Mode().IsDir() {
		fmt.Printf("Can't load swagger file at the following location %s", swaggerJsonPath)
		return err
	}

	testPlanPath := *meqaPath
	if fi, err := os.Stat(testPlanPath); os.IsNotExist(err) {
		err = os.Mkdir(testPlanPath, 0755)
		if err != nil {
			fmt.Printf("Can't create the directory at %s\n", testPlanPath)
			return err
		}
	} else if !fi.Mode().IsDir() {
		fmt.Printf("The specified location is not a directory: %s\n", testPlanPath)
		return err
	}

	// loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(swaggerJsonPath, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		return err
	}
	dag := mqswag.NewDAG()
	err = swagger.AddToDAG(dag)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		return err
	}

	dag.Sort()
	dag.CheckWeight()

	var plansToGenerate []string
	if *algorithm == algoAll {
		plansToGenerate = algoList
	} else {
		plansToGenerate = append(plansToGenerate, *algorithm)
	}

	for _, algo := range plansToGenerate {
		var testPlan *mqplan.TestSuite
		switch algo {
		case algoPath:
			testPlan, err = mqplan.GeneratePathTestPlan(swagger, dag)
			//		case algoObject:
			//			testPlan, err = mqplan.GenerateTestPlan(swagger, dag)
		default:
			testPlan, err = mqplan.GenerateSimpleTestPlan(swagger, dag)
		}
		if err != nil {
			mqutil.Logger.Printf("Error: %s", err.Error())
			return err
		}
		testPlanFile := filepath.Join(testPlanPath, algo+".yml")
		err = testPlan.DumpToFile(testPlanFile)
		if err != nil {
			mqutil.Logger.Printf("Error: %s", err.Error())
			return err
		}
		fmt.Println("Test plans generated at:", testPlanFile)
	}
	return nil
}

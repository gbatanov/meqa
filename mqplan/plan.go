package mqplan

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gbatanov/meqa/mqswag"
	"github.com/gbatanov/meqa/mqutil"
	"gopkg.in/resty.v1"
	"gopkg.in/yaml.v3"
)

const (
	StartUp = "startUp"
)

type TestParams struct {
	QueryParams  map[string]interface{} `yaml:"queryParams,omitempty"`
	FormParams   map[string]interface{} `yaml:"formParams,omitempty"`
	PathParams   map[string]interface{} `yaml:"pathParams,omitempty"`
	HeaderParams map[string]interface{} `yaml:"headerParams,omitempty"`
	BodyParams   interface{}            `yaml:"bodyParams,omitempty"`
}

// Copy the parameters from src. If there is a conflict dst will be overwritten.
func (dst *TestParams) Copy(src *TestParams) {
	dst.QueryParams = mqutil.MapCombine(dst.QueryParams, src.QueryParams)
	dst.FormParams = mqutil.MapCombine(dst.FormParams, src.FormParams)
	dst.PathParams = mqutil.MapCombine(dst.PathParams, src.PathParams)
	dst.HeaderParams = mqutil.MapCombine(dst.HeaderParams, src.HeaderParams)

	if caseMap, caseIsMap := dst.BodyParams.(map[string]interface{}); caseIsMap {
		if testMap, testIsMap := src.BodyParams.(map[string]interface{}); testIsMap {
			dst.BodyParams = mqutil.MapCombine(caseMap, testMap)
			// for map, just combine and return
			return
		}
	}
	dst.BodyParams = src.BodyParams
}

// Add the parameters from src. If there is a conflict the dst original value will be kept.
func (dst *TestParams) Add(src *TestParams) {
	dst.QueryParams = mqutil.MapAdd(dst.QueryParams, src.QueryParams)
	dst.FormParams = mqutil.MapAdd(dst.FormParams, src.FormParams)
	dst.PathParams = mqutil.MapAdd(dst.PathParams, src.PathParams)
	dst.HeaderParams = mqutil.MapAdd(dst.HeaderParams, src.HeaderParams)

	if caseMap, caseIsMap := dst.BodyParams.(map[string]interface{}); caseIsMap {
		if testMap, testIsMap := src.BodyParams.(map[string]interface{}); testIsMap {
			dst.BodyParams = mqutil.MapAdd(caseMap, testMap)
			// for map, just combine and return
			return
		}
	}

	if dst.BodyParams == nil {
		dst.BodyParams = src.BodyParams
	}
}

type TestCase struct {
	Tests []*Test
	Name  string

	// test suite parameters
	TestParams `yaml:",inline,omitempty" json:",inline,omitempty"`
	Strict     bool

	// Authentication
	Username string
	Password string
	ApiToken string

	plan *TestSuite
	db   *mqswag.DB // objects generated/obtained as part of this suite

	comment string
}

func CreateTestCase(name string, tests []*Test, plan *TestSuite) *TestCase {
	c := TestCase{}
	c.Name = name
	c.Tests = tests
	(&c.TestParams).Copy(&plan.TestParams)
	c.Strict = plan.Strict

	c.Username = plan.Username
	c.Password = plan.Password
	c.ApiToken = plan.ApiToken

	c.plan = plan
	return &c
}

// Represents all the test suites in the DSL.
type TestSuite struct {
	SuiteMap map[string](*TestCase)
	TestList [](*TestCase)
	db       *mqswag.DB
	swagger  *mqswag.Swagger
	Host     string // Если хост не прописан в swagger.yaml

	// global parameters
	TestParams `yaml:",inline,omitempty" json:",inline,omitempty"`
	Strict     bool

	// Authentication
	Username string
	Password string
	ApiToken string

	// Run result.
	resultList   []*Test
	ResultCounts map[string]int

	comment string
}

// Add a new TestCase, returns whether the Case is successfully added.
func (plan *TestSuite) Add(testCase *TestCase) error {
	if _, exist := plan.SuiteMap[testCase.Name]; exist {
		str := fmt.Sprintf("Duplicate name %s found in test plan", testCase.Name)
		mqutil.Logger.Println(str)
		return errors.New(str)
	}
	plan.SuiteMap[testCase.Name] = testCase
	plan.TestList = append(plan.TestList, testCase)
	return nil
}

func (plan *TestSuite) AddFromString(data string) error {
	var suiteMap map[string]([]*Test)
	err := yaml.Unmarshal([]byte(data), &suiteMap)
	if err != nil {
		mqutil.Logger.Printf("The following is not a valud TestCase:\n%s", data)
		return err
	}

	for suiteName, testList := range suiteMap {
		if suiteName == StartUp {
			// global parameters
			for _, t := range testList {
				t.Init(nil)
				(&plan.TestParams).Copy(&t.TestParams)
				plan.Strict = t.Strict
			}

			continue
		}
		testCase := CreateTestCase(suiteName, testList, plan)
		for _, t := range testList {
			t.Init(testCase)
		}
		err = plan.Add(testCase)
		if err != nil {
			return err
		}
	}
	return nil
}

func (plan *TestSuite) InitFromFile(path string, db *mqswag.DB) error {
	plan.Init(db.Swagger, db)

	data, err := os.ReadFile(path)
	if err != nil {
		mqutil.Logger.Printf("Can't open the following file: %s", path)
		mqutil.Logger.Println(err.Error())
		return err
	}
	chunks := strings.Split(string(data), "---")
	for _, chunk := range chunks {
		plan.AddFromString(chunk)
	}
	return nil
}

func WriteComment(comment string, f *os.File) {
	ar := strings.Split(comment, "\n")
	for _, line := range ar {
		f.WriteString("# " + line + "\n")
	}
}

// TODO: обработка panic
func (plan *TestSuite) DumpToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(plan.comment) > 0 {
		WriteComment(plan.comment, f)
	}
	for _, testCase := range plan.TestList {
		f.WriteString("\n\n")
		if len(testCase.comment) > 0 {
			WriteComment(testCase.comment, f)
		}
		_, err := f.WriteString("---\n")
		if err != nil {
			return err
		}
		testMap := map[string]interface{}{testCase.Name: testCase.Tests}
		caseBytes, err := yaml.Marshal(testMap)
		if err != nil {
			return err
		}
		count, err := f.Write(caseBytes)
		if count != len(caseBytes) || err != nil {
			panic("writing test suite failed")
		}
	}
	return nil
}

func (plan *TestSuite) WriteResultToFile(path string) error {
	// We create a new test plan that just contain all the tests in one test suite.
	p := &TestSuite{}
	tc := &TestCase{}
	// Test case name is the current time.
	tc.Name = time.Now().Format(time.RFC3339)
	p.SuiteMap = map[string]*TestCase{tc.Name: tc}
	p.TestList = append(p.TestList, tc)

	tc.Tests = append(tc.Tests, plan.resultList...)

	return p.DumpToFile(path)
}

// Вывод в консоль полного списка ошибок в тестах
func (plan *TestSuite) LogErrors() {
	fmt.Println(" ")
	fmt.Print(mqutil.AQUA)
	fmt.Printf("-----------------------------Errors----------------------------------\n")
	fmt.Print(mqutil.END)
	for _, t := range plan.resultList {
		if t.responseError != nil /*|| t.schemaError != nil */ {
			fmt.Print(mqutil.AQUA)
			fmt.Println("--------")
			fmt.Printf("%v: %v\n", t.Path, t.Name)
			fmt.Print(mqutil.END)
		}
		if t.responseError != nil {
			fmt.Print(mqutil.RED)
			fmt.Println("Response Status Code:", t.resp.StatusCode())
			if len(t.responseError.(*resty.Response).Body()) < 120 {
				fmt.Println(t.responseError)
			} else {
				re := string([]byte(t.responseError.(*resty.Response).Body())[0:120])
				re = re + "..."
				fmt.Println(re)
			}
			fmt.Print(mqutil.END)
		}
		/*
			if t.schemaError != nil {
				fmt.Print(mqutil.YELLOW)
				fmt.Println(t.schemaError.Error())
				fmt.Print(mqutil.END)
			}
		*/
	}
	fmt.Print(mqutil.AQUA)
	fmt.Println("---------------------------------------------------------------------")
	fmt.Print(mqutil.END)
}

func (plan *TestSuite) PrintSummary() {
	fmt.Print(mqutil.GREEN)
	fmt.Printf("%v: %v\n", mqutil.Passed, plan.ResultCounts[mqutil.Passed])
	fmt.Print(mqutil.RED)
	fmt.Printf("%v: %v\n", mqutil.Failed, plan.ResultCounts[mqutil.Failed])
	fmt.Print(mqutil.BLUE)
	fmt.Printf("%v: %v\n", mqutil.Skipped, plan.ResultCounts[mqutil.Skipped])
	fmt.Print(mqutil.YELLOW)
	fmt.Printf("%v: %v\n", mqutil.SchemaMismatch, plan.ResultCounts[mqutil.SchemaMismatch])
	fmt.Print(mqutil.AQUA)
	fmt.Printf("%v: %v\n", mqutil.Total, plan.ResultCounts[mqutil.Total])
	fmt.Print(mqutil.END)
}

func (plan *TestSuite) Init(swagger *mqswag.Swagger, db *mqswag.DB) {
	plan.db = db
	plan.swagger = swagger
	plan.SuiteMap = make(map[string]*TestCase)
	plan.TestList = nil
	plan.resultList = nil
}

// Run a named TestCase in the test plan.
func (plan *TestSuite) Run(name string, parentTest *Test) (map[string]int, error) {
	var err error
	tc, ok := plan.SuiteMap[name]
	resultCounts := make(map[string]int)
	if !ok || len(tc.Tests) == 0 {
		str := fmt.Sprintf("The following test suite is not found: %s", name)
		mqutil.Logger.Println(str)
		return resultCounts, errors.New(str)
	}

	tc.db = plan.db.CloneSchema()
	defer func() {
		tc.db = nil
	}()
	resultCounts[mqutil.Total] = len(tc.Tests)
	resultCounts[mqutil.Failed] = 0
	for _, test := range tc.Tests {

		if len(test.Ref) != 0 {
			test.Strict = tc.Strict
			resultCounts, err := plan.Run(test.Ref, test)
			if err != nil {
				return resultCounts, err
			}
			continue
		}

		if test.Name == StartUp {
			// Apply the parameters to the test suite.
			(&tc.TestParams).Copy(&test.TestParams)
			tc.Strict = test.Strict
			continue
		}

		dup := test.Duplicate()
		dup.Strict = tc.Strict
		if parentTest != nil {
			dup.CopyParent(parentTest)
		}
		dup.ResolveHistoryParameters(&History)
		History.Append(dup)
		if parentTest != nil {
			dup.Name = parentTest.Name // always inherit the name
		}

		err = dup.Run(tc)
		dup.err = err

		plan.resultList = append(plan.resultList, dup)
		if dup.schemaError != nil {
			resultCounts[mqutil.SchemaMismatch]++
		}
		if err != nil {
			resultCounts[mqutil.Failed]++
		} else {
			resultCounts[mqutil.Passed]++
		}
	}
	return resultCounts, err
}

// The current global TestSuite
var Current TestSuite

// TestHistory records the execution result of all the tests
type TestHistory struct {
	tests []*Test
	mutex sync.Mutex
}

// GetTest gets a test by its name
func (h *TestHistory) GetTest(name string) *Test {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	for i := len(h.tests) - 1; i >= 0; i-- {
		if h.tests[i].Name == name {
			return h.tests[i]
		}
	}
	return nil
}
func (h *TestHistory) Append(t *Test) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.tests = append(h.tests, t)
}

var History TestHistory

func init() {
	//	rand.Seed(int64(time.Now().Second()))
	rand.New(rand.NewSource(int64(time.Now().Second())))
	resty.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
}

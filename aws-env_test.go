package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/service/ssm"
)

type testCaseSlice struct {
	Name        string
	Value       string
	Path        string
	ConvertCase string

	ExpectedValue string
}

func TestPrintExportParamater(t *testing.T) {
	testCases := []testCaseSlice{
		{
			Name:        "/production/mysql_password",
			Value:       "passwerd",
			Path:        "/production",
			ConvertCase: "upper",

			ExpectedValue: "export MYSQL_PASSWORD=$'passwerd'\n",
		},
		{

			Name:        "/production/mysql_password",
			Value:       "passwerd",
			Path:        "/production",
			ConvertCase: "lower",

			ExpectedValue: "export mysql_password=$'passwerd'\n",
		},
	}
	for _, testCase := range testCases {

		fakeSSM := &ssm.Parameter{
			Name:  &testCase.Name,
			Value: &testCase.Value,
		}

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintExportParameter(testCase.Path, fakeSSM, testCase.ConvertCase)
		outC := make(chan string)
		// copy the output in a separate goroutine so printing can't block indefinitely
		go func() {
			var buf bytes.Buffer
			io.Copy(&buf, r)
			outC <- buf.String()
		}()

		// back to normal state
		w.Close()
		os.Stdout = old // restoring the real stdout
		out := <-outC

		if out != testCase.ExpectedValue {
			t.Errorf("Action: %scase failed.  We expected: %s. But we got: %s.", testCase.ConvertCase, testCase.ExpectedValue, out)
		}
	}
}

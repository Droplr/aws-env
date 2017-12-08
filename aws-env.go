package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"log"
	"os"
	"strings"
)

func main() {
	if os.Getenv("AWS_ENV_PATH") == "" {
		log.Println("aws-env running locally, without AWS_ENV_PATH")
		return
	}

	ExportVariables(os.Getenv("AWS_ENV_PATH"), "")
}

func CreateClient() *ssm.SSM {
	session := session.Must(session.NewSession())
	return ssm.New(session)
}

func ExportVariables(path string, nextToken string) {
	client := CreateClient()

	input := &ssm.GetParametersByPathInput{
		Path:           &path,
		WithDecryption: aws.Bool(true),
	}

	if nextToken != "" {
		input.SetNextToken(nextToken)
	}

	output, err := client.GetParametersByPath(input)

	if err != nil {
		log.Panic(err)
	}

	for _, element := range output.Parameters {
		PrintExportParameter(path, element)
	}

	if output.NextToken != nil {
		ExportVariables(path, *output.NextToken)
	}
}

func PrintExportParameter(path string, parameter *ssm.Parameter) {
	name := *parameter.Name
	value := *parameter.Value

	env := strings.Trim(name[len(path):], "/")
	value = strings.Replace(value, "\n", "\\n", -1)

	fmt.Printf("export %s='%s'\n", env, value)
}

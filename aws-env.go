package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"os"
	"log"
	"strings"
)

func main() {
	exportVariables(os.Getenv("AWS_ENV_PATH"))
}

func CreateClient() *ssm.SSM {
	session := session.Must(session.NewSession())
	return ssm.New(session)
}

func exportVariables(path string) {
  client := CreateClient()

	input := &ssm.GetParametersByPathInput{
		Path: &path,
		WithDecryption: aws.Bool(true),
	}

  output, err := client.GetParametersByPath(input)

	if err != nil {
	  log.Panic(err)
	}

	for _, element := range output.Parameters {
	    printExportParameter(path, element)
	}
}

func printExportParameter(path string, parameter *ssm.Parameter) {
	name := *parameter.Name
	value := *parameter.Value

	env := strings.Trim(name[len(path):], "/")

	fmt.Printf("export %s=%s \n", env, value)
}

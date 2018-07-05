package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"log"
	"os"
	"strings"
        "flag"
)

func main() {
	if os.Getenv("AWS_ENV_PATH") == "" {
		log.Println("aws-env running locally, without AWS_ENV_PATH")
		return
	}

        recursivePtr := flag.Bool("recursive", false, "recursively process parameters on path")
        flag.Parse()

	sess := CreateSession()
	client := CreateClient(sess)

	ExportVariables(client, os.Getenv("AWS_ENV_PATH"), *recursivePtr, "")
}

func CreateSession() *session.Session {
	return session.Must(session.NewSession())
}

func CreateClient(sess *session.Session) *ssm.SSM {
	return ssm.New(sess)
}

func ExportVariables(client *ssm.SSM, path string, recursive bool, nextToken string) {
        input := &ssm.GetParametersByPathInput{
                Path:           &path,
                WithDecryption: aws.Bool(true),
                Recursive: aws.Bool(recursive),
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
		ExportVariables(client, path, recursive, *output.NextToken)
	}
}

func PrintExportParameter(path string, parameter *ssm.Parameter) {
	name := *parameter.Name
	value := *parameter.Value

	env := strings.Replace(strings.Trim(name[len(path):], "/"), "/", "_", -1)
	value = strings.Replace(value, "\n", "\\n", -1)
  value = strings.Replace(value, "'", "\\'", -1)

	fmt.Printf("export %s=$'%s'\n", env, value)
}

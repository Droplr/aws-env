package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

const (
	upper = "upper"
	lower = "lower"
)

func main() {
	if os.Getenv("AWS_ENV_PATH") == "" {
		log.Println("aws-env running locally, without AWS_ENV_PATH")
		return
	}

	recursivePtr := flag.Bool("recursive", false, "recursively process parameters on path")
	convertCase := flag.String("case", "upper", "Converts ENV Key to upper or lower case")
	flag.Parse()

	if *convertCase == "upper" || *convertCase == "lower" {
	} else {
		log.Fatal("Unsupported case option. Must be 'upper' or 'lower'")
	}

	sess := CreateSession()
	client := CreateClient(sess)

	ExportVariables(client, os.Getenv("AWS_ENV_PATH"), *recursivePtr, *convertCase, "")
}

func CreateSession() *session.Session {
	return session.Must(session.NewSession())
}

func CreateClient(sess *session.Session) *ssm.SSM {
	return ssm.New(sess)
}

func ExportVariables(client *ssm.SSM, path string, recursive bool, convertCase string, nextToken string) {
	input := &ssm.GetParametersByPathInput{
		Path:           &path,
		WithDecryption: aws.Bool(true),
		Recursive:      aws.Bool(recursive),
	}

	if nextToken != "" {
		input.SetNextToken(nextToken)
	}

	output, err := client.GetParametersByPath(input)

	if err != nil {
		log.Panic(err)
	}

	for _, element := range output.Parameters {
		PrintExportParameter(path, element, convertCase)
	}

	if output.NextToken != nil {
		ExportVariables(client, path, recursive, convertCase, *output.NextToken)
	}
}

func PrintExportParameter(path string, parameter *ssm.Parameter, convertCase string) {
	name := *parameter.Name
	value := *parameter.Value

	env := strings.Replace(strings.Trim(name[len(path):], "/"), "/", "_", -1)
	value = strings.Replace(value, "\n", "\\n", -1)

	switch convertCase {
	case upper:
		env = strings.ToUpper(env)
	case lower:
		env = strings.ToLower(env)
	}
	fmt.Printf("export %s=$'%s'\n", env, value)
}

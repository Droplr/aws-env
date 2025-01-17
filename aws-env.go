package main

import (
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"log"
	"os"
	"strings"
)

const (
	formatExports = "exports"
	formatDotenv  = "dotenv"
)

func main() {
	if os.Getenv("AWS_ENV_PATH") == "" {
		log.Println("aws-env running locally, without AWS_ENV_PATH")
		return
	}

	recursivePtr := flag.Bool("recursive", false, "recursively process parameters on path")
	format := flag.String("format", formatExports, "output format")
	flag.Parse()

	if *format == formatExports || *format == formatDotenv {
	} else {
		log.Fatal("Unsupported format option. Must be 'exports' or 'dotenv'")
	}

	sess := CreateSession()
	client := CreateClient(sess)

	path := os.Getenv("AWS_ENV_PATH")
	if strings.HasSuffix(path, "/*") {
		path = strings.TrimSuffix(path, "/*")
		ExportVariables(client, path, *recursivePtr, *format, "")
	} else {
		ExportSingleVariable(client, path, *format)
	}
}

func CreateSession() *session.Session {
	return session.Must(session.NewSession())
}

func CreateClient(sess *session.Session) *ssm.SSM {
	return ssm.New(sess)
}

func ExportSingleVariable(client *ssm.SSM, name string, format string) {
	input := &ssm.GetParameterInput{
		Name:           &name,
		WithDecryption: aws.Bool(true),
	}

	output, err := client.GetParameter(input)

	if err != nil {
		log.Panic(err)
	}

	OutputParameter(output.Parameter, format)
}

func ExportVariables(client *ssm.SSM, path string, recursive bool, format string, nextToken string) {
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
		OutputParameter(element, format)
	}

	if output.NextToken != nil {
		ExportVariables(client, path, recursive, format, *output.NextToken)
	}
}

func OutputParameter(parameter *ssm.Parameter, format string) {
	name := *parameter.Name
	value := *parameter.Value

	env := name[strings.LastIndex(name, "/")+1:]
	value = strings.Replace(value, "\n", "\\n", -1)

	switch format {
	case formatExports:
		fmt.Printf("export %s=$'%s'\n", env, value)
	case formatDotenv:
		fmt.Printf("%s=\"%s\"\n", env, value)
	}
}

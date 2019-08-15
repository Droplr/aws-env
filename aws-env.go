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
	formatExports = "exports"
	formatDotenv  = "dotenv"
)

func main() {
	if os.Getenv("AWS_ENV_PATH") == "" {
		log.Fatal("[aws-env] running locally, without AWS_ENV_PATH")
	}

	if os.Getenv("AWS_REGION") == "" {
		log.Fatal("[aws-env] running locally, without AWS_REGION")
	}

	recursivePtr := flag.Bool("recursive", false, "recursively process parameters on path")
	format := flag.String("format", formatExports, "output format")
	flag.Parse()

	if *format == formatExports || *format == formatDotenv {
	} else {
		log.Fatal("[aws-env] unsupported format option; must be 'exports' or 'dotenv'")
	}

	sess := createSession()
	client := createClient(sess)

	exported := exportVariables(client, os.Getenv("AWS_ENV_PATH"), *recursivePtr, *format, "")
	if exported == 0 {
		log.Fatalf("[aws-env] no variables found at %s on %s\n", os.Getenv("AWS_ENV_PATH"), os.Getenv("AWS_REGION"))
	}

	log.Printf("[aws-env] %d variable(s) exported\n", exported)
}

func createSession() *session.Session {
	return session.Must(session.NewSession())
}

func createClient(sess *session.Session) *ssm.SSM {
	return ssm.New(sess)
}

func exportVariables(client *ssm.SSM, path string, recursive bool, format string, nextToken string) int {
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
		log.Fatalf("[aws-env] could not get parameters by path %s: %v\n", path, err)
	}

	count := 0
	for _, element := range output.Parameters {
		outputParameter(path, element, format)
		count += 1
	}

	if output.NextToken != nil {
		return count + exportVariables(client, path, recursive, format, *output.NextToken)
	}

	return count
}

func outputParameter(path string, parameter *ssm.Parameter, format string) {
	name := *parameter.Name
	value := *parameter.Value

	env := strings.Replace(strings.Trim(name[len(path):], "/"), "/", "_", -1)
	value = strings.Replace(value, "\n", "\\n", -1)

	log.Printf("[aws-env] loaded variable %s\n", env)

	switch format {
	case formatExports:
		fmt.Printf("export %s=$'%s'\n", env, value)
	case formatDotenv:
		fmt.Printf("%s=\"%s\"\n", env, value)
	}
}

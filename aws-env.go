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

type FormattingOpts struct {
	Format    string
	StripPath bool
	Uppercase bool
}

func main() {
	if os.Getenv("AWS_ENV_PATH") == "" {
		log.Println("aws-env running locally, without AWS_ENV_PATH")
		return
	}

	recursivePtr := flag.Bool("recursive", false, "recursively process parameters on path")
	stripPath := flag.Bool("strip-path", true, "remove the AWS_ENV_PATH from the environment variable name")
	uppercase := flag.Bool("uppercase", false, "print the env variable in all caps")
	format := flag.String("format", formatExports, "output format")
	flag.Parse()

	if *format == formatExports || *format == formatDotenv {
	} else {
		log.Fatal("Unsupported format option. Must be 'exports' or 'dotenv'")
	}

	sess := CreateSession()
	client := CreateClient(sess)

	formattingOpts := FormattingOpts{Format: *format, StripPath: *stripPath, Uppercase: *uppercase}
	ExportVariables(client, os.Getenv("AWS_ENV_PATH"), *recursivePtr, formattingOpts, "")
}

func CreateSession() *session.Session {
	return session.Must(session.NewSession())
}

func CreateClient(sess *session.Session) *ssm.SSM {
	return ssm.New(sess)
}

func ExportVariables(client *ssm.SSM, path string, recursive bool, opts FormattingOpts, nextToken string) {
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
		OutputParameter(path, element, opts)
	}

	if output.NextToken != nil {
		ExportVariables(client, path, recursive, opts, *output.NextToken)
	}
}

func OutputParameter(path string, parameter *ssm.Parameter, opts FormattingOpts) {
	name := *parameter.Name
	value := *parameter.Value

	if opts.StripPath {
		name = name[len(path):]
	}

	env := strings.Replace(strings.Trim(name, "/"), "/", "_", -1)

	if opts.Uppercase {
		env = strings.ToUpper(env)
	}

	value = strings.Replace(value, "\n", "\\n", -1)

	switch opts.Format {
	case formatExports:
		fmt.Printf("export %s=$'%s'\n", env, value)
	case formatDotenv:
		fmt.Printf("%s=\"%s\"\n", env, value)
	}
}

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
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
		log.Println("aws-env running locally, without AWS_ENV_PATH")
		return
	}

	recursive := flag.Bool("recursive", false, "recursively process parameters on path")
	format := flag.String("format", formatExports, "output format")
	flag.Parse()

	if *format == formatExports || *format == formatDotenv {
	} else {
		log.Fatal("Unsupported format option. Must be 'exports' or 'dotenv'")
	}

	env_paths := strings.Split(os.Getenv("AWS_ENV_PATH"), ":")

	session := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable}))
	client := ssm.New(session)

	results := make(map[string]string)

	for i := range env_paths {
		FetchParameters(client, env_paths[i], *recursive, *format, results, "")
	}

	PrintResults(results, *format)
}

func FetchParameters(client *ssm.SSM, path string, recursive bool, format string, results map[string]string, nextToken string) {
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

	for _, parameter := range output.Parameters {
		name := *parameter.Name
		value := *parameter.Value

		name = strings.Replace(strings.Trim(name[len(path):], "/"), "/", "_", -1)
		value = strings.Replace(value, "\n", "\\n", -1)

		results[name] = value
	}

	if output.NextToken != nil {
		FetchParameters(client, path, recursive, format, results, *output.NextToken)
	}
}

func PrintResults(results map[string]string, format string) {
	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	formatSpec := "export %s=$'%s'\n"
	if format == formatDotenv {
		formatSpec = "%s=\"%s\"\n"
	}

	for _, k := range keys {
		fmt.Printf(formatSpec, k, results[k])
	}
}

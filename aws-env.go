package main

import (
	"log"
	"os"
	"os/exec"
	"path"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

func main() {
	if os.Getenv("AWS_ENV_PATH") == "" {
		log.Println("aws-env running locally, without AWS_ENV_PATH")
		return
	}

	ExportVariables(os.Getenv("AWS_ENV_PATH"), "")

	binary, lookErr := exec.LookPath(os.Args[1])
	if lookErr != nil {
		panic(lookErr)
	}

	env := os.Environ()
	args := os.Args[1:]
	execErr := syscall.Exec(binary, args, env)
	if execErr != nil {
		panic(execErr)
	}
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
		SetExportParameter(element)
	}

	if output.NextToken != nil {
		ExportVariables(path, *output.NextToken)
	}
}

func SetExportParameter(parameter *ssm.Parameter) {
	name := *parameter.Name
	value := *parameter.Value

	_, envName := path.Split(name)
	os.Setenv(envName, value)
}

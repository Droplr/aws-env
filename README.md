aws-env - Secure way to handle environment variables in Docker
------------------------

aws-env is a small utility that tries to solve problem of passing environment variables to applications in a secure way, especially in Docker containers.

It uses [AWS Parameter Store](https://aws.amazon.com/ec2/systems-manager/parameter-store/) to populate environment variables while starting application inside the container.

## Usage

1. Add parameters to [Parameter Store](https://console.aws.amazon.com/ec2/v2/home#Parameters:) using hierarchy in names:
```
$ aws ssm put-parameter --name /prod/my-app/DB_USERNAME --value "Username" --type SecureString --key-id "alias/aws/ssm" --region us-west-2
$ aws ssm put-parameter --name /prod/my-app/DB_PASSWORD --value "SecretPassword" --type SecureString --key-id "alias/aws/ssm" --region us-west-2
```

2. Install aws-env (choose proper [prebuilt binary](https://github.com/Droplr/aws-env/tree/master/bin))
```
$ wget https://github.com/Droplr/aws-env/raw/master/bin/aws-env-linux-amd64 -O aws-env
```

3. Start your application with aws-env
 * `AWS_ENV_PATH` - path of parameters. If it isn't be provided, aws-env will not make requests to SSM. That way, you can run your Dockerfiles locally.
 * `AWS_REGION` and AWS Credentials - [configuring credentials](https://github.com/aws/aws-sdk-go#configuring-credentials)
```
$ AWS_ENV_PATH=/prod/my-app/ AWS_REGION=us-west-2 ./aws-env node -e "console.log(process.env)"
```


Under the hood, aws-env will export environment parameters fetched from AWS Parameter Store:

```
$ export DB_USERNAME=$'Username'
$ export DB_PASSWORD=$'SecretPassword'
```


## Example Dockerfile

```
FROM node:alpine

RUN apk update && apk upgrade && \
  apk add --no-cache openssl ca-certificates

RUN wget https://github.com/Droplr/aws-env/raw/master/bin/aws-env-linux-amd64 -O /bin/aws-env && \
  chmod +x /bin/aws-env

CMD aws-env node -e "console.log(process.env)"
```

```
$ docker build -t my-app .

$ docker run -v ~/.aws/:/root/.aws -e AWS_ENV_PATH="/prod/my-app/" -e AWS_REGION=us-west-2 -t my-app
```

For a local development, you you can still use:

```
$ docker run -t my-app
```

## Considerations

* As this script is still in development, its usage **may** change. Lock version to the
  specific commit to be sure that your Dockerfiles will work correctly!
  Example:
```
$ wget https://github.com/Droplr/aws-env/raw/befe6fa44ea508508e0bcd2c3f4ac9fc7963d542/bin/aws-env-linux-amd64
```

* You should never pass AWS credentials inside the containers, instead use IAM Roles for that -
[Managing Secrets for Amazon ECS Applications Using Parameter Store and IAM Roles for Tasks](
https://aws.amazon.com/blogs/compute/managing-secrets-for-amazon-ecs-applications-using-parameter-store-and-iam-roles-for-tasks/)

* Always use KMS for parameters encryption - store them as "SecureString"

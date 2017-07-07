aws-env - Secure way to handle environment variables in Docker
------------------------

aws-env is small utility that tries to solve problem of passing environment variables to applications in a secure way, especially in Docker containers.

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

3. Start your application:
```
$ `AWS_ENV_PATH=/prod/my-app/ AWS_REGION=us-west-2 ./aws-env` && npm run
```

Under the hood, aws-env will run:

```
$ export DB_USERNAME=Username
$ export DB_PASSWORD=SecretPassword
```

## Example Dockerfile

```
tbd
```

## Considerations

* You should never pass AWS credentials inside the containers, instead use IAM Roles for that -
[Managing Secrets for Amazon ECS Applications Using Parameter Store and IAM Roles for Tasks](
https://aws.amazon.com/blogs/compute/managing-secrets-for-amazon-ecs-applications-using-parameter-store-and-iam-roles-for-tasks/)
* Always use KMS for parameters encryption - store them as "SecureString"

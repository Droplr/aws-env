aws-env - Secure way to handle environment variables in Docker
------------------------

Forked from [Droplr/aws-env](https://github.com/Droplr/aws-env)

Published as a [docker image](https://hub.docker.com/r/base2/awsenv/)

## How it works

Searches for SSM Parameters in your AWS account based on the variables provided and places them in a .env file

```bash
$ cat /ssm/.env
export DB_HOST=$'mysql'
export DB_USERNAME=$'Username'
export DB_PASSWORD=$'SecretPassword'
```

## Parameter Hierarchy

1. `APP`: base level
2. `ENV_TYPE`: overrides `APP` parameters with the same key
3. `ENV_NAME`: top level, overrides all others

####Example:
  `/my-app/production/prod1/EMAIL` would override the value of `/my-app/EMAIL` for the prod1 environment
  `/my-app/production/API_KEY` would override the value of `/my-app/API_KEY` for the environment type production
  `/my-app/develop/test/API_KEY` would override the value of `/my-app/develop/API_KEY` for the test environment

Add parameters to [Parameter Store](https://console.aws.amazon.com/ec2/v2/home#Parameters:) using hierarchy structure:
```
$ aws ssm put-parameter --name /my-app/DB_HOST --value "mysql" --type SecureString --key-id "alias/aws/ssm" --region ap-southeast-2
$ aws ssm put-parameter --name /my-app/production/DB_USERNAME --value "Username" --type SecureString --key-id "alias/aws/ssm" --region ap-southeast-2
$ aws ssm put-parameter --name /my-app/production/prod1/DB_PASSWORD --value "SecretPassword" --type SecureString --key-id "alias/aws/ssm" --region ap-southeast-2
```

## Usage

There are 2 ways this can be implemented

1. Include `base2/awsenv` as a side car container

  * volume mount the `/ssm` directory
  * eval the `/ssm/.env` file to export the environment parameters

```yml
awsenv:
  image: base2/awsenv
  environment:
    APP: my-app
    ENV_TYPE: production
    ENV_NAME: prod1
    AWS_REGION: ap-southeast-2

test:
  image: my-app
  volumes_from:
    - awsenv
  entrypoint: eval $(cat /ssm/.env)
```

2. Build `FROM base2/awsenv as awsenv` and extract the binary

  * extract the binary from the `base2/awsenv` image to your `PATH`
  * eval the `/ssm/.env` file to export the environment parameters

```Dockerfile
FROM FROM base2/awsenv as awsenv

FROM debian:jessie

COPY --from=awsenv /awsenv /bin/awsenv

ENTRYPOINT awsenv && eval $(cat /ssm/.env)
```

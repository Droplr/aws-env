aws-env - load AWS Parameter Store values into a process environment
------------------------

**aws-env** loads parameters from [AWS SSM Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html)
into the environment of an application, typically in a Docker container. It is Mailbutler's fork of
[Droplr/aws-env](https://github.com/Droplr/aws-env), rewritten for v2 (AWS SDK for Go v2, `exec` mode, fail-closed errors).

## Usage

1. Store parameters under a path:

```
aws ssm put-parameter --name /prod/my-app/DB_USERNAME --value "Username" --type SecureString
aws ssm put-parameter --name /prod/my-app/DB_PASSWORD --value "SecretPassword" --type SecureString
```

2. Start the application through aws-env:

```
aws-env exec --path /prod/my-app/ --require DB_PASSWORD -- rails s
```

`aws-env exec` loads the parameters into the environment and then replaces itself with the command (`exec(2)`).
There is no shell and no `eval`, so values arrive exactly as stored, and signals go straight to the application.
**On any error the command is not started** and aws-env exits non-zero: missing credentials, SSM or KMS access denied,
a network problem, a timeout, a missing path, or a missing `--require` key.

In ECS, use the exec form so no shell is involved:

```json
"command": ["aws-env", "exec", "--require", "SECRET_KEY_BASE,PG_PASS", "--", "rails", "s"]
```

Credentials and region come from the default AWS chain: environment, shared config including SSO and `sso-session`
profiles, ECS task role, or EC2 instance metadata. `AWS_SDK_LOAD_CONFIG` is no longer needed.

### Flags

| Flag | Description |
| --- | --- |
| `--path P` | Parameter path. Repeatable or comma-separated; overrides `AWS_ENV_PATH`. Later paths win on duplicate names. |
| `--recursive` | Also load parameters below sub-paths. `/` in the remaining name becomes `_` (`/prod/my-app/db0/PASS` → `db0_PASS`). |
| `--require KEY,…` | Fail if one of these variables is not set after loading (from SSM or the existing environment). |
| `--no-override` | Variables already set in the environment win over SSM. |
| `--optional` | Local development only: on AWS errors (no credentials, expired SSO session, offline, access denied), warn and continue with what could be loaded. Without `kms:Decrypt`, only `String` parameters are loaded and the skipped SecureStrings are listed. In `exec` mode, a missing path is also only a warning. |
| `--timeout D` | Timeout for loading all parameters, including retries (default `15s`). |
| `--format F` | Print mode only: `exports` (default), `dotenv` or `json`. |
| `--require-path` | Print mode only: fail instead of doing nothing when no path is set. |
| `--quiet` | Only log warnings and errors. |
| `--version` | Print the version. |

Parameter names are mapped by removing the path prefix and replacing `/` with `_`. Names that are not valid
variable names (for example ones containing `-`) are skipped with a warning.

Logs go to stderr and never contain values: only paths, counts and the names of skipped, kept or missing keys.

### Local development

```
aws-env exec --optional --no-override --path /edge/ -- bundle exec rails s
```

With a valid SSO session, the SSM values are used and local variables still win. When the session has expired or you
are offline, aws-env warns and starts the command with the current environment.

### Print mode (legacy)

Without `exec`, aws-env prints the parameters instead. It is kept for existing images:

```
eval "$(AWS_ENV_PATH=/prod/my-app/ aws-env)" && rails s
```

* **exports** (default): `export KEY=$'…'`. Every byte that is special to the shell (quotes, `\`, `$`, whitespace,
  `*`, `?`, `[`, …) is escaped, so values stay exact with both `eval "$(aws-env)"` and the unquoted
  `eval $(aws-env)`. This needs a shell with `$'…'` support (bash, zsh, busybox ash), not dash.
* **dotenv**: `KEY="…"` with `\`, `"`, `$` and newlines escaped. dotenv has no single spec, so prefer `json` or `exec`
  when values must be exact.
* **json**: one object, `{"KEY": "value"}`.

On an error, aws-env exits 1 and, in `exports` format, prints `false`. The exit code alone would be lost in
`eval $(aws-env) && cmd`, but `eval false` fails, so `cmd` does not start. This does not help with `eval $(aws-env); cmd`.

If no path is set, print mode logs a message and exits 0 with no output, so the same image can run locally without AWS.
Use `--require-path` to make that an error.

## Installation

Download a release binary and check it against the release's `checksums.txt`. Pin both the version and the checksum:

```dockerfile
ARG AWS_ENV_VERSION=v2.0.1
ARG TARGETARCH
# sha256 of aws-env-linux-${TARGETARCH} from the release's checksums.txt
ARG AWS_ENV_SHA256_amd64=<sha256>
ARG AWS_ENV_SHA256_arm64=<sha256>
RUN set -eu; \
    case "$TARGETARCH" in amd64) sum="$AWS_ENV_SHA256_amd64" ;; arm64) sum="$AWS_ENV_SHA256_arm64" ;; esac; \
    wget -q -O /usr/local/bin/aws-env \
      "https://github.com/Mailbutler/aws-env/releases/download/${AWS_ENV_VERSION}/aws-env-linux-${TARGETARCH}"; \
    echo "${sum}  /usr/local/bin/aws-env" | sha256sum -c -; \
    chmod +x /usr/local/bin/aws-env
```

Binaries are built for linux and darwin on amd64 and arm64. Alternatively: `go install github.com/Mailbutler/aws-env/v2@latest`.

### Verifying a release

Releases are built by GitHub Actions with [GoReleaser](https://goreleaser.com). `checksums.txt` is signed with keyless
[cosign](https://github.com/sigstore/cosign), and every binary has a build provenance attestation:

```
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/Mailbutler/aws-env/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c checksums.txt --ignore-missing
gh attestation verify aws-env-linux-amd64 --repo Mailbutler/aws-env
```

## Upgrading from v1

* Prefer `aws-env exec -- <cmd>` over `eval $(aws-env) && <cmd>`.
* Errors now exit non-zero instead of panicking. In `exports` format, the output is then `false` (see above).
* The exports output escapes all special characters. Workarounds for mangled values (for example Base64-encoding
  JSON) are no longer needed.
* Variables with invalid names are skipped instead of producing broken output.
* Windows binaries and the `bin/` directory are gone. Download from GitHub releases instead of `raw/master/bin/...`.

## Development

```
make test       # go vet + go test -race
make snapshot   # local release build with goreleaser, unsigned
```

A release is made by pushing a `v*` tag.

## Considerations

* Don't pass AWS credentials into containers; use IAM roles for tasks.
* Store secrets as `SecureString`. The task role needs `ssm:GetParametersByPath` on the path and `kms:Decrypt` on the key.

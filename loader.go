package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

type loader struct {
	client    ssm.GetParametersByPathAPIClient
	recursive bool
	optional  bool
	log       *logger
}

// load fetches all paths in order and maps them to variable names. Later paths
// win. With optional set, a path that cannot be loaded is skipped with a warning.
func (l *loader) load(ctx context.Context, paths []string) (map[string]string, error) {
	vars := map[string]string{}
	for _, path := range paths {
		params, err := l.loadPath(ctx, path)
		if err != nil {
			if !l.optional {
				return nil, fmt.Errorf("load %s: %w", path, err)
			}
			l.log.warnf("could not load %s, continuing without it: %v", path, err)
			continue
		}

		n := 0
		for _, p := range params {
			name := envName(path, aws.ToString(p.Name))
			if !validName(name) {
				l.log.warnf("skipping %s: %q is not a valid variable name", aws.ToString(p.Name), name)
				continue
			}
			vars[name] = aws.ToString(p.Value)
			n++
		}
		l.log.infof("loaded %d parameter(s) from %s", n, path)
	}
	return vars, nil
}

func (l *loader) loadPath(ctx context.Context, path string) ([]types.Parameter, error) {
	params, err := l.fetch(ctx, path, true)
	if err == nil || !l.optional || !isAccessDenied(err) {
		return params, err
	}

	// Possibly no kms:Decrypt: retry without decryption and keep only plain
	// String parameters. If the retry also fails, the path itself is denied.
	params, retryErr := l.fetch(ctx, path, false)
	if retryErr != nil {
		return nil, err
	}
	var plain []types.Parameter
	var skipped []string
	for _, p := range params {
		if p.Type == types.ParameterTypeSecureString {
			skipped = append(skipped, aws.ToString(p.Name))
			continue
		}
		plain = append(plain, p)
	}
	if len(skipped) > 0 {
		l.log.warnf("cannot decrypt parameters in %s (%v); skipped %d SecureString(s): %s",
			path, err, len(skipped), strings.Join(skipped, ", "))
	}
	return plain, nil
}

func (l *loader) fetch(ctx context.Context, path string, decrypt bool) ([]types.Parameter, error) {
	var params []types.Parameter
	p := ssm.NewGetParametersByPathPaginator(l.client, &ssm.GetParametersByPathInput{
		Path:           aws.String(path),
		Recursive:      aws.Bool(l.recursive),
		WithDecryption: aws.Bool(decrypt),
	})
	for p.HasMorePages() {
		out, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		params = append(params, out.Parameters...)
	}
	return params, nil
}

func isAccessDenied(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "AccessDeniedException" ||
		strings.Contains(strings.ToLower(apiErr.ErrorMessage()), "kms")
}

// envName maps a parameter name to a variable name: the path prefix is
// removed and remaining "/" become "_".
func envName(path, name string) string {
	rel := strings.TrimPrefix(name, path)
	return strings.ReplaceAll(strings.Trim(rel, "/"), "/", "_")
}

var nameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validName(name string) bool { return nameRE.MatchString(name) }

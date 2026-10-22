package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// trickyValue covers everything that broke or could break the old output.
const trickyValue = "back\\slash \\n 'single' \"double\" * ? [a] $HOME `id` $(id) !x ;&|<> {a,b} ~ #c  two  spaces\ttab\nnew\nline\r\nü€ end "

// fakeSSM serves pages of parameters per path. SecureString values are
// returned as ciphertext unless decryption is requested.
type fakeSSM struct {
	params      map[string][]types.Parameter
	pageSize    int
	errs        map[string]error
	denyDecrypt map[string]bool
	calls       []ssm.GetParametersByPathInput
}

func (f *fakeSSM) GetParametersByPath(_ context.Context, in *ssm.GetParametersByPathInput, _ ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
	f.calls = append(f.calls, *in)
	path, decrypt := aws.ToString(in.Path), aws.ToBool(in.WithDecryption)
	if err := f.errs[path]; err != nil {
		return nil, err
	}
	if decrypt && f.denyDecrypt[path] {
		return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not authorized to perform: kms:Decrypt"}
	}

	var all []types.Parameter
	for _, p := range f.params[path] {
		if p.Type == types.ParameterTypeSecureString && !decrypt {
			p.Value = aws.String("AQICAHciphertext")
		}
		all = append(all, p)
	}

	size := f.pageSize
	if size == 0 {
		size = len(all) + 1
	}
	start := 0
	if in.NextToken != nil {
		start, _ = strconv.Atoi(*in.NextToken)
	}
	end := min(start+size, len(all))
	out := &ssm.GetParametersByPathOutput{Parameters: all[start:end]}
	if end < len(all) {
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	return out, nil
}

func param(name, value string) types.Parameter {
	return types.Parameter{Name: aws.String(name), Value: aws.String(value), Type: types.ParameterTypeString}
}

func secret(name, value string) types.Parameter {
	return types.Parameter{Name: aws.String(name), Value: aws.String(value), Type: types.ParameterTypeSecureString}
}

type harness struct {
	client   *fakeSSM
	env      map[string]string
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	execArgv []string
	execVars map[string]string
}

func newHarness(client *fakeSSM, environ map[string]string) *harness {
	if environ == nil {
		environ = map[string]string{}
	}
	return &harness{client: client, env: environ}
}

func (h *harness) run(args ...string) int {
	return run(args, env{
		stdout: &h.stdout,
		stderr: &h.stderr,
		lookupEnv: func(k string) (string, bool) {
			v, ok := h.env[k]
			return v, ok
		},
		newClient: func(context.Context) (ssm.GetParametersByPathAPIClient, error) {
			return h.client, nil
		},
		exec: func(argv []string, vars map[string]string) error {
			h.execArgv, h.execVars = argv, vars
			return errors.New("fake exec returned")
		},
	})
}

func TestEnvName(t *testing.T) {
	tests := []struct{ path, name, want string }{
		{"/prod/app/", "/prod/app/DB_PASS", "DB_PASS"},
		{"/prod/app", "/prod/app/DB_PASS", "DB_PASS"},
		{"/prod/app/", "/prod/app/db0/DB_PASS", "db0_DB_PASS"},
		{"/", "/FOO", "FOO"},
		{"/prod/app/", "/prod/app/my-key", "my-key"},
	}
	for _, tt := range tests {
		if got := envName(tt.path, tt.name); got != tt.want {
			t.Errorf("envName(%q, %q) = %q, want %q", tt.path, tt.name, got, tt.want)
		}
	}
}

func TestValidName(t *testing.T) {
	for _, n := range []string{"FOO", "_x", "db0_DB_PASS", "a1"} {
		if !validName(n) {
			t.Errorf("validName(%q) = false", n)
		}
	}
	for _, n := range []string{"", "1A", "my-key", "a.b", "a b", "ü"} {
		if validName(n) {
			t.Errorf("validName(%q) = true", n)
		}
	}
}

// TestShellQuoteRoundTrip evaluates the exports output in bash, both as the
// recommended `eval "$(...)"` and as the legacy unquoted `eval $(...)`, inside a
// directory with files that would match any glob in the value.
func TestShellQuoteRoundTrip(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	values := []string{
		trickyValue,
		"",
		"plain",
		"'",
		"\\",
		"*",
		"a\n\n\nb",
		"   leading and trailing   ",
		`{"type":"service_account","private_key":"-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----\n"}`,
	}

	dir := t.TempDir()
	for _, f := range []string{"a", "x", "K=", "export"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, script := range []string{
		`eval "$(cat "$1")"; printf '%s' "$K"`,
		`eval $(cat "$1"); printf '%s' "$K"`,
	} {
		for _, v := range values {
			out, err := render(map[string]string{"K": v}, formatExports)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "out.sh")
			if err := os.WriteFile(file, []byte(out), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bash, "-c", script, "bash", file)
			cmd.Dir = dir
			got, err := cmd.Output()
			if err != nil {
				t.Fatalf("%s: %v", script, err)
			}
			if string(got) != v {
				t.Errorf("%s\nvalue %q\nquoted %s\ngot   %q", script, v, out, got)
			}
		}
	}
}

func TestRenderJSON(t *testing.T) {
	vars := map[string]string{"A": trickyValue, "B": "<html>&"}
	out, err := render(vars, formatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, vars) {
		t.Errorf("got %q, want %q", got, vars)
	}
	if !strings.Contains(out, "<html>&") {
		t.Errorf("HTML characters should not be escaped: %s", out)
	}
}

func TestRenderDotenv(t *testing.T) {
	out, err := render(map[string]string{"B": "x", "A": "say \"hi\" $HOME\\n\nline"}, formatDotenv)
	if err != nil {
		t.Fatal(err)
	}
	want := "A=\"say \\\"hi\\\" \\$HOME\\\\n\\nline\"\nB=\"x\"\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestPrintExports(t *testing.T) {
	client := &fakeSSM{pageSize: 1, params: map[string][]types.Parameter{
		"/prod/app/": {param("/prod/app/B", "2"), secret("/prod/app/A", "s3cret value")},
	}}
	h := newHarness(client, map[string]string{"AWS_ENV_PATH": "/prod/app/"})
	if code := h.run(); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, h.stderr.String())
	}
	want := "export A=$'s3cret\\x20value'\nexport B=$'2'\n"
	if h.stdout.String() != want {
		t.Errorf("stdout %q, want %q", h.stdout.String(), want)
	}
	if len(client.calls) != 2 {
		t.Errorf("expected 2 paginated calls, got %d", len(client.calls))
	}
	for _, c := range client.calls {
		if !aws.ToBool(c.WithDecryption) || aws.ToBool(c.Recursive) {
			t.Errorf("unexpected input %+v", c)
		}
	}
	if strings.Contains(h.stderr.String(), "s3cret") {
		t.Errorf("value leaked to stderr: %s", h.stderr.String())
	}
}

func TestPrintWithoutPathIsNoop(t *testing.T) {
	h := newHarness(&fakeSSM{}, nil)
	if code := h.run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if h.stdout.Len() != 0 {
		t.Errorf("stdout %q", h.stdout.String())
	}
	if !strings.Contains(h.stderr.String(), "without AWS_ENV_PATH") {
		t.Errorf("stderr %q", h.stderr.String())
	}

	h = newHarness(&fakeSSM{}, nil)
	if code := h.run("--require-path"); code != 1 {
		t.Errorf("--require-path: exit %d, want 1", code)
	}
}

func TestPrintFailureBreaksEval(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	client := &fakeSSM{errs: map[string]error{"/prod/app/": denied}}

	h := newHarness(client, map[string]string{"AWS_ENV_PATH": "/prod/app/"})
	if code := h.run(); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if h.stdout.String() != "false\n" {
		t.Errorf("stdout %q, want false", h.stdout.String())
	}
	if strings.Contains(h.stderr.String(), "goroutine") {
		t.Errorf("stack trace in stderr: %s", h.stderr.String())
	}

	if bash, err := exec.LookPath("bash"); err == nil {
		cmd := exec.Command(bash, "-c", `eval $(printf '%s' "$1") && echo started`, "bash", h.stdout.String())
		if out, _ := cmd.Output(); len(out) != 0 {
			t.Errorf("command after eval ran: %q", out)
		}
	}

	h = newHarness(client, map[string]string{"AWS_ENV_PATH": "/prod/app/"})
	if code := h.run("--format", "dotenv"); code != 1 || h.stdout.Len() != 0 {
		t.Errorf("dotenv: exit %d, stdout %q", code, h.stdout.String())
	}
}

func TestPathsAndRecursive(t *testing.T) {
	client := &fakeSSM{params: map[string][]types.Parameter{
		"/shared/":   {param("/shared/A", "shared"), param("/shared/B", "shared")},
		"/prod/app/": {param("/prod/app/A", "app"), param("/prod/app/db0/PASS", "p"), param("/prod/app/bad-name", "x")},
	}}
	h := newHarness(client, map[string]string{"AWS_ENV_PATH": "/ignored/"})
	if code := h.run("--path", "/shared/", "--path=/prod/app/", "--recursive", "--format=json"); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, h.stderr.String())
	}
	var got map[string]string
	if err := json.Unmarshal(h.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "app", "B": "shared", "db0_PASS": "p"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !strings.Contains(h.stderr.String(), "bad-name") {
		t.Errorf("expected warning for invalid name, stderr: %s", h.stderr.String())
	}
	for _, c := range client.calls {
		if aws.ToString(c.Path) == "/ignored/" || !aws.ToBool(c.Recursive) {
			t.Errorf("unexpected input %+v", c)
		}
	}
}

func TestCommaSeparatedPaths(t *testing.T) {
	client := &fakeSSM{params: map[string][]types.Parameter{
		"/a/": {param("/a/X", "a")},
		"/b/": {param("/b/X", "b")},
	}}
	h := newHarness(client, map[string]string{"AWS_ENV_PATH": "/a/, /b/"})
	if code := h.run("--format=json"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.stdout.String(), `"X": "b"`) {
		t.Errorf("later path should win: %s", h.stdout.String())
	}
}

func TestExec(t *testing.T) {
	client := &fakeSSM{params: map[string][]types.Parameter{
		"/prod/app/": {secret("/prod/app/SECRET_KEY_BASE", trickyValue), param("/prod/app/PG_PASS", "pw")},
	}}
	h := newHarness(client, map[string]string{"AWS_ENV_PATH": "/prod/app/"})
	code := h.run("exec", "--require", "SECRET_KEY_BASE,PG_PASS", "--", "rails", "s", "--quiet")
	if code != exitCannot {
		t.Fatalf("exit %d, stderr: %s", code, h.stderr.String())
	}
	if !reflect.DeepEqual(h.execArgv, []string{"rails", "s", "--quiet"}) {
		t.Errorf("argv %q", h.execArgv)
	}
	if h.execVars["SECRET_KEY_BASE"] != trickyValue || h.execVars["PG_PASS"] != "pw" {
		t.Errorf("vars %q", h.execVars)
	}
	if h.stdout.Len() != 0 {
		t.Errorf("exec mode must not print: %q", h.stdout.String())
	}
}

func TestExecFailsClosed(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	tests := []struct {
		name   string
		client *fakeSSM
		env    map[string]string
		args   []string
	}{
		{"ssm denied", &fakeSSM{errs: map[string]error{"/p/": denied}}, map[string]string{"AWS_ENV_PATH": "/p/"}, nil},
		{"kms denied", &fakeSSM{denyDecrypt: map[string]bool{"/p/": true}}, map[string]string{"AWS_ENV_PATH": "/p/"}, nil},
		{"network", &fakeSSM{errs: map[string]error{"/p/": context.DeadlineExceeded}}, map[string]string{"AWS_ENV_PATH": "/p/"}, nil},
		{"no path", &fakeSSM{}, nil, nil},
		{"required missing", &fakeSSM{}, map[string]string{"AWS_ENV_PATH": "/p/"}, []string{"--require", "PG_PASS"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(tt.client, tt.env)
			args := append(append([]string{"exec"}, tt.args...), "--", "rails", "s")
			if code := h.run(args...); code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if h.execArgv != nil {
				t.Errorf("command was started")
			}
		})
	}
}

func TestExecAWSConfigError(t *testing.T) {
	h := newHarness(&fakeSSM{}, map[string]string{"AWS_ENV_PATH": "/p/"})
	e := env{
		stdout: &h.stdout, stderr: &h.stderr,
		lookupEnv: func(k string) (string, bool) { v, ok := h.env[k]; return v, ok },
		newClient: func(context.Context) (ssm.GetParametersByPathAPIClient, error) {
			return nil, errors.New("failed to load SSO token")
		},
		exec: func(argv []string, _ map[string]string) error { h.execArgv = argv; return errors.New("x") },
	}
	if code := run([]string{"exec", "--", "true"}, e); code != 1 || h.execArgv != nil {
		t.Errorf("exit %d, started %v", code, h.execArgv)
	}
	if code := run([]string{"exec", "--optional", "--", "true"}, e); code != exitCannot || h.execArgv == nil {
		t.Errorf("--optional: exit %d, started %v", code, h.execArgv)
	}
}

func TestOptional(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	client := &fakeSSM{
		params: map[string][]types.Parameter{
			"/kms/": {param("/kms/PLAIN", "p"), secret("/kms/SECRET", "s")},
		},
		denyDecrypt: map[string]bool{"/kms/": true},
		errs:        map[string]error{"/denied/": denied},
	}
	h := newHarness(client, map[string]string{"LOCAL": "1"})
	code := h.run("exec", "--optional", "--path", "/denied/,/kms/", "--require", "LOCAL", "--", "bundle", "exec", "rails")
	if code != exitCannot {
		t.Fatalf("exit %d, stderr: %s", code, h.stderr.String())
	}
	if !reflect.DeepEqual(h.execVars, map[string]string{"PLAIN": "p"}) {
		t.Errorf("vars %q", h.execVars)
	}
	stderr := h.stderr.String()
	for _, want := range []string{"could not load /denied/", "skipped 1 SecureString(s): /kms/SECRET"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}

	h = newHarness(&fakeSSM{}, nil)
	if code := h.run("exec", "--optional", "--", "true"); code != exitCannot || h.execArgv == nil {
		t.Errorf("--optional without path: exit %d", code)
	}
}

func TestNoOverride(t *testing.T) {
	client := &fakeSSM{params: map[string][]types.Parameter{
		"/p/": {param("/p/A", "ssm"), param("/p/B", "ssm")},
	}}
	h := newHarness(client, map[string]string{"AWS_ENV_PATH": "/p/", "A": "local"})
	if code := h.run("exec", "--no-override", "--", "env"); code != exitCannot {
		t.Fatalf("exit %d", code)
	}
	if !reflect.DeepEqual(h.execVars, map[string]string{"B": "ssm"}) {
		t.Errorf("vars %q", h.execVars)
	}

	h = newHarness(client, map[string]string{"AWS_ENV_PATH": "/p/", "A": "local"})
	if code := h.run("exec", "--", "env"); code != exitCannot || h.execVars["A"] != "ssm" {
		t.Errorf("without --no-override SSM should win: %q", h.execVars)
	}
}

func TestQuiet(t *testing.T) {
	client := &fakeSSM{params: map[string][]types.Parameter{"/p/": {param("/p/A", "1"), param("/p/a-b", "1")}}}
	h := newHarness(client, map[string]string{"AWS_ENV_PATH": "/p/"})
	if code := h.run("--quiet"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if s := h.stderr.String(); strings.Contains(s, "loaded") || !strings.Contains(s, "warning") {
		t.Errorf("stderr %q", s)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--format", "yaml"},
		{"exec"},
		{"exec", "--format", "json", "--", "x"},
		{"rails"},
		{"--timeout", "0s"},
		{"--nope"},
	} {
		h := newHarness(&fakeSSM{}, map[string]string{"AWS_ENV_PATH": "/p/"})
		if code := h.run(args...); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, exitUsage)
		}
	}
}

func TestVersion(t *testing.T) {
	h := newHarness(&fakeSSM{}, nil)
	if code := h.run("--version"); code != 0 || strings.TrimSpace(h.stdout.String()) == "" {
		t.Errorf("exit %d, stdout %q", code, h.stdout.String())
	}
}

// TestExecCommand runs the real execCommand in a child process and checks
// that the value reaches the new program byte for byte.
func TestExecCommand(t *testing.T) {
	if os.Getenv("AWS_ENV_TEST_EXEC") == "1" {
		err := execCommand([]string{"printenv", "K"}, map[string]string{"K": trickyValue})
		t.Fatalf("execCommand returned: %v", err)
	}
	if _, err := exec.LookPath("printenv"); err != nil {
		t.Skip("printenv not available")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestExecCommand$")
	cmd.Env = append(os.Environ(), "AWS_ENV_TEST_EXEC=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != trickyValue+"\n" {
		t.Errorf("got %q", out)
	}
}

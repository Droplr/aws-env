// Command aws-env loads parameters from AWS SSM Parameter Store into the
// environment of a process.
//
//	aws-env exec [flags] -- <cmd> [args...]   run <cmd> with the parameters in its environment
//	aws-env [flags]                           print the parameters (legacy, for eval / .env files)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = ""

const (
	formatExports = "exports"
	formatDotenv  = "dotenv"
	formatJSON    = "json"
)

const (
	exitOK       = 0
	exitFailure  = 1
	exitUsage    = 2
	exitCannot   = 126
	exitNotFound = 127
)

type options struct {
	exec        bool
	paths       []string
	recursive   bool
	format      string
	require     []string
	requirePath bool
	noOverride  bool
	optional    bool
	quiet       bool
	showVersion bool
	timeout     time.Duration
	command     []string
}

// env abstracts the process environment and side effects so run can be tested.
type env struct {
	stdout    io.Writer
	stderr    io.Writer
	lookupEnv func(string) (string, bool)
	newClient func(context.Context) (ssm.GetParametersByPathAPIClient, error)
	exec      func(argv []string, vars map[string]string) error
}

func main() {
	os.Exit(run(os.Args[1:], env{
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		lookupEnv: os.LookupEnv,
		newClient: newSSMClient,
		exec:      execCommand,
	}))
}

func newSSMClient(ctx context.Context) (ssm.GetParametersByPathAPIClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return ssm.NewFromConfig(cfg), nil
}

func run(args []string, e env) int {
	log := &logger{w: e.stderr}

	opts, err := parseArgs(args, e)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		log.errorf("%v", err)
		return exitUsage
	}
	log.quiet = opts.quiet

	if opts.showVersion {
		fmt.Fprintln(e.stdout, versionString())
		return exitOK
	}

	vars, err := resolve(opts, e, log)
	if errors.Is(err, errRunLocally) {
		// Legacy behaviour: without a path, print mode is a no-op so the same
		// image can run locally without AWS.
		log.infof("aws-env running locally, without AWS_ENV_PATH")
		return exitOK
	}
	if err != nil {
		log.errorf("%v", err)
		if !opts.exec && opts.format == formatExports {
			// The exit code is lost in `eval $(aws-env) && cmd`; make eval fail instead.
			fmt.Fprintln(e.stdout, "false")
		}
		return exitFailure
	}

	if !opts.exec {
		out, err := render(vars, opts.format)
		if err != nil {
			log.errorf("%v", err)
			return exitFailure
		}
		if _, err := io.WriteString(e.stdout, out); err != nil {
			log.errorf("write output: %v", err)
			return exitFailure
		}
		return exitOK
	}

	err = e.exec(opts.command, vars)
	log.errorf("exec %s: %v", opts.command[0], err)
	if errors.Is(err, exec.ErrNotFound) {
		return exitNotFound
	}
	return exitCannot
}

var (
	errNoPath     = errors.New("no parameter path: set AWS_ENV_PATH or pass --path")
	errRunLocally = errors.New("no parameter path in print mode")
)

// resolve loads the parameters and applies --no-override and --require. It
// returns the variables that should be added to the environment.
func resolve(opts *options, e env, log *logger) (map[string]string, error) {
	vars := map[string]string{}

	switch {
	case len(opts.paths) > 0:
		ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
		defer cancel()

		client, err := e.newClient(ctx)
		if err != nil {
			if !opts.optional {
				return nil, fmt.Errorf("aws config: %w", err)
			}
			log.warnf("could not load AWS config, continuing without SSM parameters: %v", err)
			break
		}
		l := &loader{client: client, recursive: opts.recursive, optional: opts.optional, log: log}
		if vars, err = l.load(ctx, opts.paths); err != nil {
			return nil, err
		}
	case opts.exec && opts.optional:
		log.warnf("no parameter path set, continuing without SSM parameters")
	case opts.exec || opts.requirePath:
		return nil, errNoPath
	default:
		return nil, errRunLocally
	}

	if opts.noOverride {
		var kept []string
		for k := range vars {
			if _, ok := e.lookupEnv(k); ok {
				kept = append(kept, k)
				delete(vars, k)
			}
		}
		if len(kept) > 0 {
			sort.Strings(kept)
			log.infof("kept %d existing variable(s): %s", len(kept), strings.Join(kept, ", "))
		}
	}

	var missing []string
	for _, k := range opts.require {
		if _, ok := vars[k]; ok {
			continue
		}
		if _, ok := e.lookupEnv(k); ok {
			continue
		}
		missing = append(missing, k)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("required variable(s) missing: %s", strings.Join(missing, ", "))
	}

	return vars, nil
}

func parseArgs(args []string, e env) (*options, error) {
	opts := &options{}
	name := "aws-env"
	if len(args) > 0 && args[0] == "exec" {
		opts.exec = true
		name = "aws-env exec"
		args = args[1:]
	}

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	fs.Usage = func() { usage(fs, opts.exec) }

	var paths, require listFlag
	fs.Var(&paths, "path", "parameter path, repeatable or comma-separated; later paths win (default $AWS_ENV_PATH)")
	fs.BoolVar(&opts.recursive, "recursive", false, "recursively load parameters below each path")
	fs.Var(&require, "require", "comma-separated variables that must be set after loading")
	fs.BoolVar(&opts.noOverride, "no-override", false, "variables already set in the environment win")
	fs.BoolVar(&opts.optional, "optional", false, "on AWS errors, warn and continue with what could be loaded (local development only)")
	fs.BoolVar(&opts.requirePath, "require-path", false, "fail if no parameter path is set")
	fs.DurationVar(&opts.timeout, "timeout", 15*time.Second, "timeout for loading all parameters")
	fs.BoolVar(&opts.quiet, "quiet", false, "only log warnings and errors")
	fs.BoolVar(&opts.showVersion, "version", false, "print the version and exit")
	if !opts.exec {
		fs.StringVar(&opts.format, "format", formatExports, "output format: exports, dotenv or json")
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	opts.paths = paths
	opts.require = require

	if opts.showVersion {
		return opts, nil
	}

	if opts.exec {
		opts.command = fs.Args()
		if len(opts.command) == 0 {
			return nil, errors.New("missing command: aws-env exec [flags] -- <cmd> [args...]")
		}
	} else if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument %q (did you mean `aws-env exec -- %s`?)", fs.Arg(0), strings.Join(fs.Args(), " "))
	}

	switch opts.format {
	case "", formatExports, formatDotenv, formatJSON:
	default:
		return nil, fmt.Errorf("unsupported format %q: must be exports, dotenv or json", opts.format)
	}
	if opts.timeout <= 0 {
		return nil, errors.New("--timeout must be positive")
	}

	if len(opts.paths) == 0 {
		if v, ok := e.lookupEnv("AWS_ENV_PATH"); ok {
			opts.paths = splitList(v)
		}
	}

	return opts, nil
}

func usage(fs *flag.FlagSet, execMode bool) {
	w := fs.Output()
	if execMode {
		fmt.Fprintln(w, "Usage: aws-env exec [flags] -- <cmd> [args...]")
		fmt.Fprintln(w, "\nLoads SSM parameters into the environment and replaces itself with <cmd>.")
	} else {
		fmt.Fprintln(w, "Usage: aws-env exec [flags] -- <cmd> [args...]")
		fmt.Fprintln(w, "       aws-env [flags]")
		fmt.Fprintln(w, "\nWithout a command, prints the parameters for `eval \"$(aws-env)\"` or a .env file.")
	}
	fmt.Fprintln(w, "\nFlags:")
	fs.PrintDefaults()
}

func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

// listFlag is a repeatable flag that also accepts comma-separated values.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	*l = append(*l, splitList(v)...)
	return nil
}

func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// logger writes to stderr only. It must never be given parameter values.
type logger struct {
	w     io.Writer
	quiet bool
}

func (l *logger) infof(format string, args ...any) {
	if !l.quiet {
		fmt.Fprintf(l.w, "aws-env: "+format+"\n", args...)
	}
}

func (l *logger) warnf(format string, args ...any) {
	fmt.Fprintf(l.w, "aws-env: warning: "+format+"\n", args...)
}

func (l *logger) errorf(format string, args ...any) {
	fmt.Fprintf(l.w, "aws-env: error: "+format+"\n", args...)
}

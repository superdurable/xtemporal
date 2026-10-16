package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func envFile(path, key string) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return "", e
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.Trim(strings.TrimPrefix(line, key+"="), "\"'"), nil
		}
	}
	return "", fmt.Errorf("%s absent from %s", key, path)
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: limitless-verify probe|setup|locktest|fence|check|report [flags]")
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	target := fs.String("target", "pg16", "pg16 or limitless")
	env := fs.String("env", ".env", "DSN file")
	out := fs.String("out", "results", "result directory")
	iterations := fs.Int("iterations", 50, "locktest repetitions")
	a9 := fs.Int("a9-checks", 100000, "read after write checks")
	shards := fs.Int("shards", 64, "shard IDs")
	only := fs.String("cases", "", "comma-separated locktest case names to run (default all, A9 runs only when included or unset)")
	writers := fs.Int("writers-per-owner", 4, "writers per owner")
	duration := fs.Duration("duration", 10*time.Minute, "fence duration")
	minWait := fs.Duration("takeover-min", 50*time.Millisecond, "minimum takeover interval")
	maxWait := fs.Duration("takeover-max", 500*time.Millisecond, "maximum takeover interval")
	minTakeovers := fs.Int("min-takeovers", 100, "minimum takeovers per shard")
	kill := fs.Bool("kill-conns", false, "terminate random writer connections")
	cancel := fs.Float64("cancel-rate", 0, "fraction of operations canceled")
	if e := fs.Parse(os.Args[2:]); e != nil {
		return e
	}
	if *target != "pg16" && *target != "limitless" {
		return fmt.Errorf("invalid target %q", *target)
	}
	key := "PG16_DSN"
	if *target == "limitless" {
		key = "LIMITLESS_DSN"
	}
	dsn, e := envFile(*env, key)
	if e != nil {
		return e
	}
	c, e := newConnector(*target, dsn)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(*out, 0755); e != nil {
		return e
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), max(2*time.Minute, *duration+2*time.Minute))
	defer cancelFn()
	switch cmd {
	case "probe":
		return probe(ctx, c, *shards)
	case "setup":
		return setup(ctx, c, *shards)
	case "locktest":
		return locktest(ctx, c, *out, *iterations, *a9, *shards, *only)
	case "fence":
		return fence(ctx, c, *out, *shards, *writers, *duration, *minWait, *maxWait, *kill, *cancel)
	case "check":
		return check(ctx, c, *out, *shards, *minTakeovers)
	case "report":
		return report(*target, *out)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func latest(out, target, suffix string) (string, error) {
	files, e := filepath.Glob(filepath.Join(out, target+"-*"+suffix))
	if e != nil {
		return "", e
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no %s results", suffix)
	}
	return files[len(files)-1], nil
}

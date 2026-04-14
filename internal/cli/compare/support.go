package comparecmd

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func PrintHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl compare <old-url> <new-url> [--backend chromium|lightpanda] [--viewport <width>x<height>] [--wait-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare --old-session <id> --new-session <id> [--wait-selector <css>] [--wait-function <js>] [--wait-network-idle] [--wait-timeout <ms>] [--ignore-text-regex <regex>]... [--ignore-selector <rule>]... [--mask-selector <rule>]... [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "   or: nxctl compare --manifest <file> [--continue-on-error] [--limit <n>] [--output-json <file>] [--output-md <file>] [--json]")
	fmt.Fprintln(w, "rules: @eN, role=<value>, name=<value>, text=<value>, testid=<value>, href=<value>, role=<value>&name=<value>")
}

func isHelpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help")
}

func parseCommandFlags(fs *flag.FlagSet, args []string, stderr io.Writer, command string) error {
	normalized := normalizeFlagArgs(fs, args)
	output := fs.Output()
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	defer fs.SetOutput(output)

	if err := fs.Parse(normalized); err != nil {
		message := strings.TrimSpace(buf.String())
		if message != "" {
			fmt.Fprintln(stderr, message)
		}
		fmt.Fprintf(stderr, "hint: run `nxctl help %s` for details\n", command)
		return err
	}

	return nil
}

func normalizeFlagArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		name, hasValue := parseFlagToken(arg)
		flags = append(flags, arg)
		if hasValue {
			continue
		}

		defined := fs.Lookup(name)
		if defined == nil || isBoolFlag(defined) {
			continue
		}
		if i+1 >= len(args) {
			continue
		}

		flags = append(flags, args[i+1])
		i++
	}

	return append(flags, positionals...)
}

func parseFlagToken(arg string) (string, bool) {
	trimmed := strings.TrimLeft(arg, "-")
	if trimmed == "" {
		return "", false
	}
	if index := strings.IndexByte(trimmed, '='); index >= 0 {
		return trimmed[:index], true
	}
	return trimmed, false
}

func isBoolFlag(def *flag.Flag) bool {
	if def == nil {
		return false
	}
	getter, ok := def.Value.(flag.Getter)
	if !ok {
		return false
	}
	_, ok = getter.Get().(bool)
	return ok
}

func resolvedViewport(value string) (int, int, error) {
	if strings.TrimSpace(value) == "" {
		return defaultViewportWidth, defaultViewportHeight, nil
	}
	return parseViewport(value)
}

func parseViewport(value string) (int, int, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	parts := strings.Split(normalized, "x")
	if len(parts) != 2 {
		return 0, 0, errors.New("viewport must be WIDTHxHEIGHT")
	}

	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return 0, 0, errors.New("viewport width must be a positive integer")
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return 0, 0, errors.New("viewport height must be a positive integer")
	}

	return width, height, nil
}

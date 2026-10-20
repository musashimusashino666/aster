// Command aster is the official CLI for the Aster programming language.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/codegen"
	"github.com/aster-lang/aster/internal/parser"
	"github.com/aster-lang/aster/internal/toolchain"
	"github.com/aster-lang/aster/internal/vm"
)

//go:embed VERSION
var versionFile string

var version = strings.TrimSpace(versionFile)

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "aster:", err)
		os.Exit(1)
	}
}
func dispatch(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return fmt.Errorf("missing command")
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil
	case "version", "--version":
		fmt.Printf("Aster %s (early prototype)\n", version)
		return nil
	case "check":
		return commandCheck(args[1:])
	case "run":
		return commandRun(args[1:])
	case "compile":
		return commandCompile(args[1:])
	case "exec":
		return commandExec(args[1:])
	case "build":
		return commandBuild(args[1:])
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func usage(w io.Writer) {
	fmt.Fprintln(w, `Aster — a statically typed, multi-backend programming language (prototype)

Usage:
  aster check <file.ast>                 Parse and type-check source
  aster run [--engine vm|native] <file>  Execute source (VM by default)
  aster compile [-o file] <file.ast>     Compile to versioned Aster bytecode JSON
  aster exec <file.abc.json>             Execute compiled bytecode
  aster build [--target native] [-o output] <file.ast>  Build a native executable
  aster version                          Print version

Examples:
  aster run examples/hello.ast
  aster check examples/fibonacci.ast
  aster compile -o fibonacci.abc.json examples/fibonacci.ast
  aster exec fibonacci.abc.json
  aster build -o ./fibonacci examples/fibonacci.ast
  aster run --engine native examples/hello.ast

The native backend currently uses the host Go toolchain. Structs/enums, modules,
package registries, WebAssembly, async/await, and a production-grade standard library
are not implemented in this release.`)
}
func readSource(path string) (string, *checker.TypedProgram, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", path, err)
	}
	tree, err := parser.Parse(string(b), path)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", path, err)
	}
	typed, err := checker.Analyze(tree)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", path, err)
	}
	return string(b), typed, nil
}
func commandCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: aster check <file.ast>")
	}
	_, _, err := readSource(fs.Arg(0))
	if err == nil {
		fmt.Printf("%s: OK\n", fs.Arg(0))
	}
	return err
}

// flagsBeforePositionals normalizes value-taking options so the CLI accepts
// common forms such as "aster run file.ast --engine native" as well as placing
// options before the positional file argument. The standard flag package stops
// parsing options at the first positional argument.
func flagsBeforePositionals(args []string, valueFlags ...string) []string {
	var options, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		matched := false
		for _, name := range valueFlags {
			if arg == name {
				options = append(options, arg)
				if i+1 < len(args) {
					options = append(options, args[i+1])
					i++
				}
				matched = true
				break
			}
			if strings.HasPrefix(arg, name+"=") {
				options = append(options, arg)
				matched = true
				break
			}
		}
		if !matched {
			positionals = append(positionals, arg)
		}
	}
	return append(options, positionals...)
}

func commandRun(args []string) error {
	args = flagsBeforePositionals(args, "--engine", "-engine")
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	engine := fs.String("engine", "vm", "execution engine: vm or native")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: aster run [--engine vm|native] <file.ast>")
	}
	_, typed, err := readSource(fs.Arg(0))
	if err != nil {
		return err
	}
	switch *engine {
	case "vm":
		bc, err := bytecode.Compile(typed.Tree)
		if err != nil {
			return err
		}
		return vm.Run(bc, os.Stdout)
	case "native":
		src, err := codegen.GenerateTyped(typed)
		if err != nil {
			return err
		}
		return toolchain.RunNative(src)
	default:
		return fmt.Errorf("unknown engine %q (choose vm or native)", *engine)
	}
}
func commandCompile(args []string) error {
	args = flagsBeforePositionals(args, "-o")
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("o", "", "output bytecode path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: aster compile [-o file.abc.json] <file.ast>")
	}
	_, typed, err := readSource(fs.Arg(0))
	if err != nil {
		return err
	}
	bc, err := bytecode.Compile(typed.Tree)
	if err != nil {
		return err
	}
	path := *out
	if path == "" {
		base := strings.TrimSuffix(fs.Arg(0), filepath.Ext(fs.Arg(0)))
		path = base + ".abc.json"
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	encErr := bytecode.Write(f, bc)
	closeErr := f.Close()
	if encErr != nil {
		return encErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Printf("compiled %s -> %s\n", fs.Arg(0), path)
	return nil
}
func commandExec(args []string) error {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: aster exec <file.abc.json>")
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	bc, err := bytecode.Read(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return vm.Run(bc, os.Stdout)
}
func commandBuild(args []string) error {
	args = flagsBeforePositionals(args, "-o", "--target")
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("o", "", "output executable path")
	target := fs.String("target", "native", "build target")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: aster build [-o output] <file.ast>")
	}
	if *target != "native" {
		return fmt.Errorf("target %q is not implemented; only --target native is supported in Aster %s", *target, version)
	}
	_, typed, err := readSource(fs.Arg(0))
	if err != nil {
		return err
	}
	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		return err
	}
	output := *out
	if output == "" {
		base := strings.TrimSuffix(filepath.Base(fs.Arg(0)), filepath.Ext(fs.Arg(0)))
		output = base
		if filepath.Ext(output) == "" && os.PathSeparator == '\\' {
			output += ".exe"
		}
	}
	if err = toolchain.BuildNative(goSource, output); err != nil {
		return err
	}
	fmt.Printf("built %s\n", output)
	return nil
}

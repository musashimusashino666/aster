// Command aster is the official CLI for the Aster programming language.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/codegen"
	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/formatter"
	"github.com/aster-lang/aster/internal/parser"
	"github.com/aster-lang/aster/internal/resource"
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
	case "fmt":
		return commandFmt(args[1:])
	case "exec":
		return commandExec(args[1:])
	case "build":
		return commandBuild(args[1:])
	case "__resource-launcher":
		return commandResourceLauncher(args[1:])
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func usage(w io.Writer) {
	fmt.Fprintln(w, `Aster — a statically typed, multi-backend programming language (prototype)

Usage:
  aster check <file.ast>                 Parse and type-check source
  aster run [--engine vm|native] [resource flags] <file>  Execute source (VM by default)
  aster compile [-o file] <file.ast>     Compile to versioned Aster bytecode JSON
  aster exec <file.abc.json>             Execute compiled bytecode
  aster fmt [--check] <file.ast>          Format source or check its formatting
  aster build [--target native] [-o output] <file.ast>  Build a native executable
  aster version                          Print version

Security:
  Native --resource-profile constrained uses Linux cgroup v2 when delegated.
  Without required controls, constrained execution fails before Aster code runs.
  Resource limits are not a filesystem/network sandbox. Native execution runs with host privileges and is not a sandbox. Do not
  execute untrusted Aster source with --engine native or run untrusted native
  binaries built from Aster source.

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
// options before the positional file argument. A standalone "--" ends this
// normalization pass so dash-prefixed filenames and other following arguments
// remain positional. The standard flag package stops parsing options at the
// first positional argument.
func flagsBeforePositionals(args []string, valueFlags ...string) []string {
	return flagsBeforePositionalsAndBooleans(args, nil, valueFlags...)
}

// flagsBeforePositionalsAndBooleans moves recognized options before positional
// arguments without consuming a filename after a standalone boolean flag.
func flagsBeforePositionalsAndBooleans(args []string, booleanFlags []string, valueFlags ...string) []string {
	var options, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i:]...)
			break
		}
		matched := false
		for _, name := range booleanFlags {
			if arg == name || strings.HasPrefix(arg, name+"=") {
				options = append(options, arg)
				matched = true
				break
			}
		}
		if !matched {
			for _, name := range valueFlags {
				if arg == name {
					options = append(options, arg)
					if i+1 < len(args) && args[i+1] != "--" {
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
		}
		if !matched {
			positionals = append(positionals, arg)
		}
	}
	return append(options, positionals...)
}

func commandRun(args []string) error {
	args = flagsBeforePositionals(args,
		"--engine", "-engine",
		"--resource-profile", "--memory-limit", "--cpu-rate", "--cpu-time-limit")
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	engine := fs.String("engine", "vm", "execution engine: vm or native")
	resourceProfile := fs.String("resource-profile", "none", "resource profile: none or constrained")
	memoryLimit := fs.String("memory-limit", "", "OS-accounted memory limit (for example 256MiB)")
	cpuRate := fs.String("cpu-rate", "", "maximum CPU throughput in logical CPUs (for example 0.5 or 1.0)")
	cpuTimeLimit := fs.String("cpu-time-limit", "", "cumulative cgroup CPU-time budget (for example 30s)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: aster run [--engine vm|native] [--resource-profile constrained --memory-limit 256MiB] <file.ast>")
	}

	var err error
	limits := resource.Limits{}
	constrained := false
	hasResourceOption := *memoryLimit != "" || *cpuRate != "" || *cpuTimeLimit != ""
	switch *resourceProfile {
	case "none":
		if hasResourceOption {
			return fmt.Errorf("resource limit flags require --resource-profile constrained")
		}
	case "constrained":
		constrained = true
		if *engine != "native" {
			return fmt.Errorf("--resource-profile constrained requires --engine native")
		}
		if *memoryLimit != "" {
			limits.MemoryMaxBytes, err = resource.ParseMemoryBytes(*memoryLimit)
			if err != nil {
				return err
			}
		}
		if *cpuRate != "" {
			limits.CPUQuota, err = strconv.ParseFloat(*cpuRate, 64)
			if err != nil {
				return fmt.Errorf("invalid --cpu-rate %q: %w", *cpuRate, err)
			}
			if limits.CPUQuota <= 0 {
				return fmt.Errorf("--cpu-rate must be greater than zero")
			}
		}
		if *cpuTimeLimit != "" {
			limits.CPUTimeLimit, err = time.ParseDuration(*cpuTimeLimit)
			if err != nil {
				return fmt.Errorf("invalid --cpu-time-limit %q: %w", *cpuTimeLimit, err)
			}
			if limits.CPUTimeLimit <= 0 {
				return fmt.Errorf("--cpu-time-limit must be greater than zero")
			}
		}
		if err := limits.Validate(); err != nil {
			return fmt.Errorf("invalid constrained resource limits: %w", err)
		}
	default:
		return fmt.Errorf("unknown resource profile %q (choose none or constrained)", *resourceProfile)
	}

	var nativeCtx context.Context
	var cancelNative context.CancelFunc
	if *engine == "native" {
		nativeCtx, cancelNative = context.WithTimeout(context.Background(), toolchain.NativeRunTimeout)
		defer cancelNative()
	}
	_, typed, err := readSource(fs.Arg(0))
	if err != nil {
		return err
	}
	if nativeCtx != nil {
		if err := nativeCtx.Err(); err != nil {
			return diagnostic.WrapToolchain(fmt.Errorf("native run deadline exceeded during source analysis: %w", err))
		}
	}
	switch *engine {
	case "vm":
		bc, err := bytecode.CompileTyped(typed)
		if err != nil {
			return err
		}
		return vm.Run(bc, os.Stdout)
	case "native":
		src, err := codegen.GenerateTypedContext(nativeCtx, typed)
		if err != nil {
			return err
		}
		if constrained {
			return toolchain.RunNativeConstrainedContext(nativeCtx, src, limits)
		}
		return toolchain.RunNativeContext(nativeCtx, src)
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
	bc, err := bytecode.CompileTyped(typed)
	if err != nil {
		return err
	}
	path := *out
	if path == "" {
		base := strings.TrimSuffix(fs.Arg(0), filepath.Ext(fs.Arg(0)))
		path = base + ".abc.json"
	}
	if err := writeBytecodeAtomically(path, bc); err != nil {
		return err
	}
	fmt.Printf("compiled %s -> %s\n", fs.Arg(0), path)
	return nil
}

// writeBytecodeAtomically commits a fully encoded bytecode file only after
// validation, encoding, syncing, and closing have all succeeded.
func writeBytecodeAtomically(path string, program *bytecode.Program) error {
	return writeOutputAtomically(path, func(w io.Writer) error {
		return bytecode.Write(w, program)
	})
}

// writeOutputAtomically writes to a temporary file on the destination filesystem
// and renames it into place only after the writer has succeeded. A failed write
// cannot truncate or leave a partial replacement of an existing output file.
// writeOutputAtomically replaces path by an atomic rename after a complete write.
// If path is a symbolic link, the rename replaces the link itself rather than
// following it; the link target is left unchanged.
func writeOutputAtomically(path string, write func(io.Writer) error) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory %s: %w", dir, err)
	}
	mode := os.FileMode(0)
	preserveMode := false
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		preserveMode = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output %s: %w", path, err)
	}

	tempDir, err := os.MkdirTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary output directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempPath := filepath.Join(tempDir, base)
	file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	if preserveMode {
		if err := file.Chmod(mode); err != nil {
			_ = file.Close()
			return fmt.Errorf("preserve output permissions: %w", err)
		}
	}

	if err := write(file); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary output: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync temporary output: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary output: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace output %s: %w", path, err)
	}
	return nil
}

// commandFmt formats one source file in place, or checks it without writing
// when --check is supplied. Symlink paths are resolved so formatting updates
// the target file rather than replacing the symlink itself.
func commandFmt(args []string) error {
	args = flagsBeforePositionalsAndBooleans(args, []string{"--check"})
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	check := fs.Bool("check", false, "check formatting without changing the file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: aster fmt [--check] <file.ast>")
	}
	path := fs.Arg(0)
	source, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	formatted, err := formatter.Format(string(source))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if *check {
		if string(source) != formatted {
			return fmt.Errorf("%s: source is not formatted (run aster fmt %s)", path, path)
		}
		return nil
	}
	if string(source) == formatted {
		return nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve source %s: %w", path, err)
	}
	if err := writeOutputAtomically(target, func(w io.Writer) error {
		_, err := io.WriteString(w, formatted)
		return err
	}); err != nil {
		return err
	}
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
		return diagnostic.WrapBytecode(err, fs.Arg(0))
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
	ctx, cancel := context.WithTimeout(context.Background(), toolchain.NativeBuildTimeout)
	defer cancel()
	_, typed, err := readSource(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return diagnostic.WrapToolchain(fmt.Errorf("native build deadline exceeded during source analysis: %w", err))
	}
	goSource, err := codegen.GenerateTypedContext(ctx, typed)
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
	if err = toolchain.BuildNativeContext(ctx, goSource, output); err != nil {
		return err
	}
	fmt.Printf("built %s\n", output)
	return nil
}

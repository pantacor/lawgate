package inventory

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/gosource"
)

// GoCommand describes the build inputs of a direct go run, test or build.
// Workspace is empty for automatic selection and "off" to disable workspaces.
type GoCommand struct {
	Dir         string
	Workspace   string
	ModFile     string
	Mod         string
	GOOS        string
	GOARCH      string
	Tags        string
	Subcommand  string
	Operands    []string
	BuildFlags  []string
	testArgs    []string
	biteFlags   []string
	environment []string
	executable  string
	initialDir  string
}

// ParseGoCommand uses module as the initial working directory. A zero
// Subcommand identifies an opaque command.
func ParseGoCommand(module string, argv []string) (GoCommand, error) {
	prefix, args := goInvocation(argv)
	command := GoCommand{Dir: module, initialDir: module}
	if len(args) == 0 {
		return command, nil
	}
	command.executable, command.Subcommand = args[0], args[1]
	if err := command.parseEnvironment(prefix); err != nil {
		return command, err
	}
	if err := command.parseArgs(args[2:]); err != nil {
		return command, err
	}
	if len(command.Operands) == 0 && command.Subcommand != "run" {
		command.Operands = []string{"."}
	}
	return command, command.validateOperands()
}

func goInvocation(args []string) (prefix, command []string) {
	index := 0
	if len(args) != 0 && filepath.Base(args[0]) == "env" {
		index++
		for index < len(args) && strings.Contains(args[index], "=") {
			index++
		}
	}
	if len(args)-index < 2 || filepath.Base(args[index]) != "go" {
		return nil, nil
	}
	return args[:index], args[index:]
}

func (command *GoCommand) parseEnvironment(prefix []string) error {
	if len(prefix) == 0 {
		return nil
	}
	for _, assignment := range prefix[1:] {
		name, value, _ := strings.Cut(assignment, "=")
		if err := command.setVariable(name, value); err != nil {
			return err
		}
		command.environment = append(command.environment, assignment)
	}
	return nil
}

func (command *GoCommand) setVariable(name, value string) error {
	switch name {
	case "GOOS":
		command.GOOS = value
	case "GOARCH":
		command.GOARCH = value
	case "CGO_ENABLED":
	case "GOWORK":
		special := value == "off" || value == ""
		if !special && !filepath.IsAbs(value) {
			return fmt.Errorf("GOWORK must be off or an absolute workspace path")
		}
		command.Workspace = value
	default:
		return fmt.Errorf("unsupported Go environment prefix %s", name)
	}
	return nil
}

func (command *GoCommand) parseArgs(args []string) error {
	switch command.Subcommand {
	case "run", "test", "build":
	default:
		return fmt.Errorf("unsupported Go subcommand %q", command.Subcommand)
	}
	index := 0
	for index < len(args) {
		arg := args[index]
		if command.argsEnd(arg) {
			return command.finishArgs(args[index:])
		}
		operand := !strings.HasPrefix(arg, "-")
		if operand && command.Subcommand != "test" {
			command.Operands = positionalOperands(command.Subcommand, args[index:])
			return command.afterOperands(args[index+len(command.Operands):])
		}
		if operand {
			command.Operands = positionalOperands(command.Subcommand, args[index:])
			index += len(command.Operands)
			continue
		}
		count, err := command.parseFlag(args[index:], index)
		if err != nil {
			return err
		}
		index += count
	}
	return nil
}

func (command GoCommand) afterOperands(args []string) error {
	if command.Subcommand == "build" && len(args) > 0 {
		return fmt.Errorf("Go build flag %s must precede package operands", args[0])
	}
	return nil
}

func (command GoCommand) validateOperands() error {
	for _, operand := range command.Operands {
		if err := validateOperand(operand); err != nil {
			return err
		}
	}
	return nil
}

func validateOperand(operand string) error {
	if strings.Contains(operand, "@") {
		return fmt.Errorf("version-qualified Go operand %q bypasses the repository module", operand)
	}
	if operand == "..." {
		return nil
	}
	base, _ := strings.CutSuffix(operand, "/...")
	if strings.Contains(base, "...") {
		return fmt.Errorf("unsupported Go package wildcard %q: use a final /...", operand)
	}
	return nil
}

func (command *GoCommand) finishArgs(args []string) error {
	if command.Subcommand == "test" {
		command.testArgs = args
		return nil
	}
	if args[0] != "--" {
		return nil
	}
	if len(args) == 1 {
		return nil
	}
	command.Operands = positionalOperands(command.Subcommand, args[1:])
	return command.afterOperands(args[1+len(command.Operands):])
}

func (command GoCommand) argsEnd(arg string) bool {
	testArgs := command.Subcommand == "test" && (arg == "-args" || arg == "--args")
	afterTestFlags := command.Subcommand == "test" && len(command.Operands) != 0
	literalTestArg := afterTestFlags && !strings.HasPrefix(arg, "-")
	terminator := testArgs || arg == "--"
	return terminator || literalTestArg
}

func (command *GoCommand) parseFlag(args []string, index int) (int, error) {
	flag, err := readGoFlag(command.Subcommand, args)
	if err != nil {
		return 0, err
	}
	if err := command.setFlag(flag.name, flag.value, index); err != nil {
		return 0, err
	}
	command.keepFlag(flag, args[:flag.words])
	return flag.words, nil
}

// goFlag is one flag of the command line; words counts the arguments it spans.
type goFlag struct {
	name  string
	value string
	kind  flagKind
	words int
}

func readGoFlag(subcommand string, args []string) (goFlag, error) {
	raw, value, assigned := strings.Cut(args[0], "=")
	name := "-" + strings.TrimPrefix(strings.TrimPrefix(raw, "-"), "-")
	flag := goFlag{name: name, value: value, kind: goFlagKind(subcommand, name), words: 1}
	if flag.kind == unsupportedFlag {
		return flag, fmt.Errorf("unsupported Go flag %s", name)
	}
	takesValue := flag.kind == buildValue || flag.kind == testValue
	if !takesValue || assigned {
		return flag, nil
	}
	if len(args) < 2 {
		return flag, fmt.Errorf("Go flag %s requires a value", name)
	}
	flag.value, flag.words = args[1], 2
	return flag, nil
}

func (command *GoCommand) keepFlag(flag goFlag, words []string) {
	if flag.kind == buildValue || flag.kind == buildSwitch {
		command.BuildFlags = append(command.BuildFlags, words...)
	}
	if command.Subcommand == "test" && keepsBiteFlag(flag.name) {
		command.biteFlags = append(command.biteFlags, words...)
	}
}

// A bite rerun is a probe, not the build of record. It sets its own
// selection, evidence format and cache policy, and it must not write the
// profiles, traces or binaries the configured command writes.
func keepsBiteFlag(name string) bool {
	name = "-" + strings.TrimPrefix(name[1:], "test.")
	switch name {
	case "-C", "-tags", "-json", "-count", "-run", "-skip", "-list", "-bench", "-fuzz":
		return false
	case "-coverprofile", "-cpuprofile", "-memprofile", "-blockprofile", "-mutexprofile",
		"-trace", "-outputdir", "-o", "-c":
		return false
	}
	return true
}

func (command *GoCommand) setFlag(name, value string, index int) error {
	switch name {
	case "-C":
		if index != 0 || value == "" {
			return fmt.Errorf("Go flag -C must be first and name a directory")
		}
		command.Dir = commandPath(command.Dir, value)
	case "-modfile":
		if filepath.Ext(value) != ".mod" {
			return fmt.Errorf("Go flag -modfile requires a .mod file")
		}
		command.ModFile = commandPath(command.Dir, value)
	case "-mod":
		if !validModMode(value) {
			return fmt.Errorf("unsupported Go flag -mod value %q", value)
		}
		command.Mod = value
	case "-tags":
		command.Tags = value
	}
	return nil
}

func validModMode(mode string) bool {
	switch mode {
	case "mod", "readonly", "vendor":
		return true
	}
	return false
}

func commandPath(dir, name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(dir, name)
}

type flagKind int

const (
	unsupportedFlag flagKind = iota
	buildValue
	buildSwitch
	testValue
	otherSwitch
)

func goFlagKind(subcommand, name string) flagKind {
	switch name {
	case "-mod", "-modfile", "-tags":
		return buildValue
	case "-race", "-asan", "-msan", "-trimpath", "-cover":
		return buildSwitch
	case "-C", "-p", "-covermode":
		return testValue
	case "-v", "-x", "-a", "-work":
		return otherSwitch
	}
	if subcommand == "build" && name == "-o" {
		return testValue
	}
	if subcommand == "test" {
		return testFlagKind(name)
	}
	return unsupportedFlag
}

func testFlagKind(name string) flagKind {
	name = "-" + strings.TrimPrefix(name[1:], "test.")
	switch name {
	case "-run", "-skip", "-bench", "-benchtime", "-count", "-cpu", "-list",
		"-parallel", "-shuffle", "-timeout", "-fuzz", "-fuzztime", "-fuzzminimizetime",
		"-blockprofile", "-blockprofilerate", "-coverprofile", "-cpuprofile", "-memprofile",
		"-memprofilerate", "-mutexprofile", "-mutexprofilefraction", "-outputdir", "-trace":
		return testValue
	case "-json", "-v", "-short", "-failfast", "-benchmem", "-fullpath":
		return otherSwitch
	}
	return unsupportedFlag
}

func positionalOperands(subcommand string, args []string) []string {
	if subcommand == "run" {
		return runOperands(args)
	}
	end := 1
	for end < len(args) && !strings.HasPrefix(args[end], "-") {
		end++
	}
	return args[:end]
}

func runOperands(args []string) []string {
	if !strings.HasSuffix(args[0], ".go") {
		return args[:1]
	}
	end := 1
	for end < len(args) && strings.HasSuffix(args[end], ".go") {
		end++
	}
	return args[:end]
}

// GoTestBuildFlags supplies the build flags of a direct Go test override.
func (module Module) GoTestBuildFlags() []string {
	command, err := ParseGoCommand(module.Path, module.TestCommand)
	if err != nil || command.Subcommand != "test" {
		return nil
	}
	return command.BuildFlags
}

// GoCommandOperands returns the package or file operands of a direct invocation.
func GoCommandOperands(args []string) []string {
	command, err := ParseGoCommand(".", args)
	if err != nil {
		return nil
	}
	return command.Operands
}

func (command GoCommand) listArgs(selection string, targets []string) []string {
	executable := command.executable
	if executable == "" {
		executable = "go"
	} else if filepath.Base(executable) != executable {
		executable = commandPath(command.initialDir, executable)
	}
	args := []string{executable, "list", "-e", selection, "-json=" + gosource.PackageFields}
	if selection == "-deps" && command.Subcommand == "test" {
		args = append(args, "-test")
	}
	return append(append(args, command.BuildFlags...), targets...)
}

// SelectTest rebuilds the configured command for a single test.
func (command GoCommand) SelectTest(name, tag string, operands []string) []string {
	var args []string
	if len(command.environment) > 0 {
		args = append([]string{"env"}, command.environment...)
	}
	args = append(args, command.executable, "test")
	if command.Dir != command.initialDir {
		args = append(args, "-C", command.Dir)
	}
	args = append(args, "-json", "-count=1", "-run", "^"+regexp.QuoteMeta(name)+"$")
	args = append(args, command.biteFlags...)
	tags := strings.FieldsFunc(command.Tags, tagSeparator)
	if !slices.Contains(tags, tag) {
		tags = append(tags, tag)
	}
	args = append(args, "-tags="+strings.Join(tags, ","))
	args = append(args, operands...)
	return append(args, command.testArgs...)
}

func tagSeparator(r rune) bool { return r == ' ' || r == ',' }

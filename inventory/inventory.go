// Package inventory loads the repository's expected law checks.
package inventory

import (
	"encoding/json"
	"fmt"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

const Filename = "lawgate.json"

const DefaultBaseline = ".lawgate-readability"

const DefaultLock = "LAWS.lock"

type Readability struct {
	Baseline string   `json:"baseline"`
	Include  []string `json:"include,omitempty"`
	Exclude  []string `json:"exclude,omitempty"`
}

// RulePackage adds repository-specific Go declarations to the rule vocabulary.
type RulePackage struct {
	Path  string   `json:"path"`
	Names []string `json:"names,omitempty"`
}

type Config struct {
	Lock         string        `json:"lock,omitempty"`
	Product      string        `json:"product,omitempty"`
	Owner        string        `json:"owner,omitempty"`
	Platform     string        `json:"platform,omitempty"`
	Readability  *Readability  `json:"readability,omitempty"`
	Modules      []Module      `json:"modules"`
	GateFiles    []string      `json:"gate_files,omitempty"`
	RulePackages []RulePackage `json:"rule_packages,omitempty"`
	// OutsideIncludes are absolute directories outside the repository the
	// owner approves for cgo include directories and headers: no
	// CODEOWNERS entry can cover them, so lawgate notes them instead of
	// failing.
	OutsideIncludes []string `json:"outside_includes,omitempty"`
	// RequireApproval makes rule-changes check the owner's approval of the
	// pull or merge request behind every rule-file change. rule-changes
	// reads it at BASE, so a change cannot switch it off for itself.
	RequireApproval bool `json:"require_approval,omitempty"`
}

// Command overrides are argv arrays. Machine commands receive -laws -junit FILE.
// Go and Node test overrides must emit go test -json and Node TAP, respectively.
type Module struct {
	Path           string          `json:"path"`
	Language       string          `json:"language"`
	Laws           string          `json:"laws"`
	Machines       *bool           `json:"machines"`
	MachineCommand []string        `json:"machine_command,omitempty"`
	MachineBuild   *GoMachineBuild `json:"machine_build,omitempty"`
	GenerateCheck  []string        `json:"generate_check,omitempty"`
	TestCommand    []string        `json:"test_command,omitempty"`
	SharedLaws     []string        `json:"shared_laws,omitempty"`
	RequiredTests  []string        `json:"required_tests,omitempty"`
	TimeoutSeconds *int            `json:"timeout_seconds,omitempty"`
}

func Load(root string) (Config, error) {
	file, err := os.Open(filepath.Join(root, Filename))
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	return Decode(file)
}

func Decode(reader io.Reader) (Config, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("%s: %w", Filename, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return Config{}, fmt.Errorf("%s: expected exactly one JSON object", Filename)
	}
	return config, config.validate()
}

func (config Config) validate() error {
	if len(config.Modules) == 0 {
		return fmt.Errorf("%s: no modules", Filename)
	}
	if err := config.validateModules(); err != nil {
		return err
	}
	if err := config.validateGateFiles(); err != nil {
		return err
	}
	return config.validateOptions()
}

// validateModules checks every module and refuses a repeated module path.
func (config Config) validateModules() error {
	seen := make(map[string]bool)
	for _, module := range config.Modules {
		if err := module.validate(); err != nil {
			return err
		}
		if seen[module.Path] {
			return fmt.Errorf("%s: duplicate module %q", Filename, module.Path)
		}
		seen[module.Path] = true
	}
	return nil
}

func (config Config) validateGateFiles() error {
	for _, file := range config.GateFiles {
		if !validPath(file) || file == "." {
			return fmt.Errorf("%s: invalid gate file %q", Filename, file)
		}
	}
	return nil
}

// LockPath returns the repository-relative path of the proof lock.
func (config Config) LockPath() string {
	if config.Lock == "" {
		return DefaultLock
	}
	return config.Lock
}

func (config Config) validateOptions() error {
	if config.Lock != "" && !validLockPath(config.Lock) {
		return fmt.Errorf("%s: invalid proof lock path %q", Filename, config.Lock)
	}
	if err := config.validatePlatform(); err != nil {
		return err
	}
	if err := config.validateRulePackages(); err != nil {
		return err
	}
	if err := config.validateOutsideIncludes(); err != nil {
		return err
	}
	return config.validateReadability()
}

func validLockPath(name string) bool {
	reserved := name == "." || name == Filename || path.Base(name) == "LAWS.md"
	gitPath := name == ".git" || strings.HasPrefix(name, ".git/")
	available := !reserved && !gitPath
	return validPath(name) && available
}

// validateOutsideIncludes refuses an outside include directory that is
// not a clean absolute path, or that is listed twice.
func (config Config) validateOutsideIncludes() error {
	seen := make(map[string]bool)
	for _, directory := range config.OutsideIncludes {
		if !validOutsideDirectory(directory) {
			return fmt.Errorf("%s: invalid outside include directory %q: it must be a clean absolute path other than /", Filename, directory)
		}
		if seen[directory] {
			return fmt.Errorf("%s: duplicate outside include directory %q", Filename, directory)
		}
		seen[directory] = true
	}
	return nil
}

func validOutsideDirectory(directory string) bool {
	absolute := path.IsAbs(directory) && path.Clean(directory) == directory
	below := absolute && directory != "/"
	cleanCharacters := !strings.ContainsAny(directory, "\\\x00\r\n")
	return below && cleanCharacters
}

// CheckOutsideIncludes refuses an approved outside include directory that
// is the repository root, root, or holds it: it would approve every
// repository file, and neighbouring checkouts nobody owns, without review.
// An entry is compared as written and, when it exists, with its symbolic
// links resolved.
func (config Config) CheckOutsideIncludes(root string) error {
	root = filepath.ToSlash(root)
	for _, directory := range config.OutsideIncludes {
		if holdsRoot(directory, root) {
			return fmt.Errorf("%s: outside include directory %q holds the repository %s: approve a directory outside it", Filename, directory, root)
		}
	}
	return nil
}

func holdsRoot(directory, root string) bool {
	spellings := []string{directory}
	if resolved, err := filepath.EvalSymlinks(directory); err == nil {
		spellings = append(spellings, filepath.ToSlash(resolved))
	}
	for _, spelling := range spellings {
		prefix := strings.TrimSuffix(spelling, "/") + "/"
		if root == spelling || strings.HasPrefix(root, prefix) {
			return true
		}
	}
	return false
}

// validatePlatform refuses an unknown platform and an owner without one.
func (config Config) validatePlatform() error {
	if config.Platform != "" && !ValidPlatform(config.Platform) {
		return fmt.Errorf("%s: unsupported platform %q", Filename, config.Platform)
	}
	if config.Owner != "" && config.Platform == "" {
		return fmt.Errorf("%s: owner requires a platform", Filename)
	}
	if config.RequireApproval && config.Owner == "" {
		return fmt.Errorf("%s: require_approval requires an owner and a platform", Filename)
	}
	return nil
}

func (config Config) validateRulePackages() error {
	for _, rule := range config.RulePackages {
		if err := rule.validate(); err != nil {
			return fmt.Errorf("%s: %w", Filename, err)
		}
	}
	return nil
}

// validateReadability refuses a baseline outside the repository and a glob
// that path.Match cannot parse. An absent section is valid.
func (config Config) validateReadability() error {
	if config.Readability == nil {
		return nil
	}
	baseline := config.Readability.Baseline
	if baseline == "." || !validPath(baseline) {
		return fmt.Errorf("%s: invalid readability baseline %q", Filename, baseline)
	}
	patterns := slices.Concat(config.Readability.Include, config.Readability.Exclude)
	for _, pattern := range patterns {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("%s: invalid readability glob %q: %w", Filename, pattern, err)
		}
	}
	return nil
}

// validate refuses a rule package the classifier could never match: an
// import path that is empty, padded or not slash-separated, or a name that
// is not a Go identifier. An empty names list means every declaration.
func (rule RulePackage) validate() error {
	if !validImportPath(rule.Path) {
		return fmt.Errorf("invalid rule package path %q", rule.Path)
	}
	for _, name := range rule.Names {
		if !token.IsIdentifier(name) {
			return fmt.Errorf("rule package %q: invalid rule package name %q", rule.Path, name)
		}
	}
	return nil
}

func validImportPath(importPath string) bool {
	unsafe := strings.ContainsFunc(importPath, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("\"'`\\", r)
	})
	slashSeparated := fs.ValidPath(importPath) && importPath != "."
	return !unsafe && slashSeparated
}

func ValidPlatform(platform string) bool {
	return platform == "github" || platform == "gitlab"
}

func (module Module) validate() error {
	if err := module.validateIdentity(); err != nil {
		return err
	}
	if err := module.validateMachines(); err != nil {
		return err
	}
	if err := module.validateTimeout(); err != nil {
		return err
	}
	if err := module.checkMachineBuild(); err != nil {
		return err
	}
	if err := module.validateSharedLaws(); err != nil {
		return err
	}
	if err := module.validateGoCommands(); err != nil {
		return err
	}
	return module.validateRequiredTests()
}

// validateIdentity refuses an unusable module path, laws path or language.
func (module Module) validateIdentity() error {
	switch {
	case !validPath(module.Path):
		return fmt.Errorf("invalid module path %q", module.Path)
	case !validPath(module.Laws) || module.Laws == ".":
		return fmt.Errorf("module %q: invalid laws path %q", module.Path, module.Laws)
	case module.Language != "go" && module.Language != "node":
		return fmt.Errorf("module %q: unsupported language %q", module.Path, module.Language)
	}
	return nil
}

// validateMachines refuses an inconsistent machines flag and machine command.
// It runs before any check that dereferences the machines flag.
func (module Module) validateMachines() error {
	switch {
	case module.Machines == nil:
		return fmt.Errorf("module %q: machines must be true or false", module.Path)
	case !*module.Machines && len(module.MachineCommand) > 0:
		return fmt.Errorf("module %q: machine command requires machines", module.Path)
	case module.missingNodeMachineCommand():
		return fmt.Errorf("module %q: missing machine command", module.Path)
	}
	return nil
}

const maxTimeoutSeconds = 24 * 60 * 60

func (module Module) validTimeout() bool {
	if module.TimeoutSeconds == nil {
		return true
	}
	seconds := *module.TimeoutSeconds
	return seconds > 0 && seconds <= maxTimeoutSeconds
}

func (module Module) validateTimeout() error {
	if module.validTimeout() {
		return nil
	}
	return fmt.Errorf("module %q: timeout_seconds must be from 1 to %d", module.Path, maxTimeoutSeconds)
}

// Timeout bounds each command run for this module.
func (module Module) Timeout() time.Duration {
	if module.TimeoutSeconds == nil {
		return CommandTimeout
	}
	return time.Duration(*module.TimeoutSeconds) * time.Second
}

func (module Module) validateGoCommands() error {
	for _, args := range module.Commands() {
		if _, err := ParseGoCommand(module.Path, args); err != nil {
			return fmt.Errorf("module %q: %w", module.Path, err)
		}
	}
	return nil
}

func (module Module) validateRequiredTests() error {
	for _, name := range module.RequiredTests {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n") {
			return fmt.Errorf("module %q: invalid required test %q", module.Path, name)
		}
	}
	return nil
}

func (module Module) validateSharedLaws() error {
	seen := map[string]bool{module.Laws: true}
	for _, name := range module.SharedLaws {
		if !validPath(name) || name == "." {
			return fmt.Errorf("module %q: invalid shared laws path %q", module.Path, name)
		}
		if seen[name] {
			return fmt.Errorf("module %q: duplicate law document %q", module.Path, name)
		}
		seen[name] = true
	}
	return nil
}

func (module Module) missingNodeMachineCommand() bool {
	machineModule := module.Language == "node" && *module.Machines
	return machineModule && len(module.MachineCommand) == 0
}

func validPath(name string) bool {
	relative := name != "" && !path.IsAbs(name) && path.Clean(name) == name
	inside := name != ".." && !strings.HasPrefix(name, "../")
	cleanCharacters := !strings.ContainsAny(name, "\\\x00\r\n")
	valid := relative && inside && cleanCharacters
	return valid
}

func (module Module) MachineArgs(report string) []string {
	command := module.MachineCommand
	if len(command) == 0 {
		command = defaultMachineCommand()
	}
	return append(slices.Clone(command), "-laws", "-junit", report)
}

func defaultMachineCommand() []string {
	return []string{"go", "run", "./cmd/machine"}
}

func (module Module) TestArgs() []string {
	if len(module.TestCommand) > 0 {
		return module.TestCommand
	}
	if module.Language == "node" {
		return []string{"node", "--test", "--test-reporter=tap"}
	}
	return []string{"go", "test", "-json", "-count=1", "./..."}
}

// Commands lists command entry points for ownership discovery. The implicit
// test runner draws its rule seeds from the named enforcers.
func (module Module) Commands() [][]string {
	var commands [][]string
	if len(module.TestCommand) != 0 {
		commands = append(commands, module.TestCommand)
	}
	if *module.Machines {
		commands = append(commands, module.MachineArgs(""))
	}
	if len(module.GenerateCheck) != 0 {
		commands = append(commands, module.GenerateCheck)
	}
	return commands
}

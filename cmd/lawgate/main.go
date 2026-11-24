package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/pantacor/lawgate/bite"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/rulechanges"
	"github.com/pantacor/lawgate/templates"
)

func main() {
	if _, set := os.LookupEnv(bite.Env); set {
		fmt.Fprintf(os.Stderr, "%s is set in the gate's environment. It is reserved for enforcer reruns.\n", bite.Env)
		os.Exit(2)
	}
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("usage: lawgate init|check|lock|review|readability-baseline|rule-files|codeowners|check-codeowners|rule-changes")
	}
	switch arguments[0] {
	case "init":
		return initCommand(arguments[1:])
	case "readability-baseline":
		return baselineCommand(arguments[1:])
	case "check":
		return checkCommand(arguments[1:], output)
	case "lock":
		return lockCommand(arguments[1:], output)
	case "rule-files", "codeowners", "check-codeowners":
		return ownersCommand(arguments[0], arguments[1:], output)
	case "rule-changes":
		return changesCommand(arguments[1:], output)
	case "review":
		return reviewCommand(arguments[1:], output)
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

// ownerOptions are the flags of rule-files, codeowners and check-codeowners.
type ownerOptions struct {
	root     string
	owner    string
	platform string
	base     string
	fallback bool
}

// ownerFlags are the flags each ownership command uses. A flag a command
// would ignore is not defined for it, so passing one is an error.
var ownerFlags = map[string][]string{
	"rule-files":       {"root"},
	"codeowners":       {"root", "owner", "platform", "fallback"},
	"check-codeowners": {"root", "owner", "platform", "base"},
}

func parseOwnerOptions(action string, arguments []string) (ownerOptions, error) {
	var options ownerOptions
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	for _, name := range ownerFlags[action] {
		options.define(flags, name)
	}
	if err := flags.Parse(arguments); err != nil {
		return options, err
	}
	if flags.NArg() != 0 {
		return options, fmt.Errorf("%s accepts flags only", action)
	}
	return options, nil
}

func (options *ownerOptions) define(flags *flag.FlagSet, name string) {
	switch name {
	case "root":
		flags.StringVar(&options.root, "root", ".", "repository root")
	case "owner":
		flags.StringVar(&options.owner, "owner", "", "required CODEOWNER (default: lawgate.json owner)")
	case "platform":
		flags.StringVar(&options.platform, "platform", "", "github or gitlab (default: lawgate.json platform, else github)")
	case "base":
		flags.StringVar(&options.base, "base", "", "also check the active CODEOWNERS at this base ref, with the base's owner")
	case "fallback":
		flags.BoolVar(&options.fallback, "fallback", false, "start with the platform's '* OWNER' block before the exact entries")
	}
}

func ownersCommand(action string, arguments []string, output io.Writer) error {
	options, err := parseOwnerOptions(action, arguments)
	if err != nil {
		return err
	}
	// Files that could not be classified are listed (protected) and reported.
	listed, classifyErr := rulechanges.RuleFiles(options.root)
	files := listed.Files
	if files == nil {
		return classifyErr
	}
	printNotes(listed.Notes)
	switch action {
	case "rule-files":
		return errors.Join(printLines(output, files), classifyErr)
	case "codeowners":
		return errors.Join(writeOwnersBlock(output, files, options), classifyErr)
	default:
		return errors.Join(checkOwnersCoverage(files, options), classifyErr)
	}
}

func printLines(output io.Writer, lines []string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(output, line); err != nil {
			return err
		}
	}
	return nil
}

func writeOwnersBlock(output io.Writer, files []string, options ownerOptions) error {
	policy, err := resolveOwnerPolicy(options)
	if err != nil {
		return err
	}
	if options.fallback {
		err = writeFallback(output, policy.platform, policy.owner)
	}
	if err != nil {
		return err
	}
	return rulechanges.WriteOwners(output, files, policy.owner)
}

// writeFallback writes the platform's catch-all block for owner (GitLab's
// inside the mandatory [Law gate] section). Put before the generated
// entries, it covers a new rule file until the next generated block lists it.
func writeFallback(output io.Writer, platform, owner string) error {
	if err := rulechanges.ValidOwner(owner); err != nil {
		return err
	}
	_, err := io.WriteString(output, templates.CodeownersFallback(platform, owner))
	return err
}

func checkOwnersCoverage(files []string, options ownerOptions) error {
	policy, err := resolveOwnerPolicy(options)
	if err != nil {
		return err
	}
	current := rulechanges.CheckOwners(options.root, files, policy.owner, policy.platform)
	if options.base == "" {
		return current
	}
	atBase := rulechanges.CheckOwnersAt(options.root, options.base, files, policy.owner, policy.platform)
	return errors.Join(current, atBase)
}

type ownerPolicy struct {
	owner    string
	platform string
}

// resolveOwnerPolicy takes the owner from --owner, else from lawgate.json:
// at --base when one is given, so a candidate cannot name its own reviewer
// by editing lawgate.json; otherwise from the worktree. No owner is built in.
// --owner replaces only the owner: the platform still comes from --platform,
// else lawgate.json, else github.
func resolveOwnerPolicy(options ownerOptions) (ownerPolicy, error) {
	config, err := ownerConfig(options)
	if err != nil {
		return ownerPolicy{}, fmt.Errorf("CODEOWNER from %s: %w (or pass --owner)", inventory.Filename, err)
	}
	owner := cmp.Or(options.owner, config.Owner)
	policy := ownerPolicy{owner: owner, platform: cmp.Or(options.platform, config.Platform, "github")}
	if !inventory.ValidPlatform(policy.platform) {
		return policy, fmt.Errorf("platform %q: want github or gitlab", policy.platform)
	}
	return policy, rulechanges.ValidOwner(policy.owner)
}

// ownerConfig is the lawgate.json the policy reads. With --owner, a missing
// file is no error: the owner is given and the platform defaults.
func ownerConfig(options ownerOptions) (inventory.Config, error) {
	config, err := loadOwnerConfig(options)
	missing := errors.Is(err, fs.ErrNotExist)
	if missing && options.owner != "" {
		return inventory.Config{}, nil
	}
	return config, err
}

func loadOwnerConfig(options ownerOptions) (inventory.Config, error) {
	if options.base != "" {
		return rulechanges.ConfigAt(options.root, options.base)
	}
	return inventory.Load(options.root)
}

func changesCommand(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("rule-changes", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return errors.New("usage: lawgate rule-changes [--root DIR] BASE HEAD")
	}
	return rulechanges.Audit(*root, flags.Arg(0), flags.Arg(1), output)
}

// printNotes shows ownership notes (approved outside include directories,
// system headers) on standard error, so command output stays parseable.
func printNotes(notes []string) {
	for _, note := range notes {
		fmt.Fprintln(os.Stderr, "note:", note)
	}
}

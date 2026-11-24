package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/rulechanges"
	"github.com/pantacor/lawgate/templates"
)

type starterFile struct {
	Name string
	Data []byte
}

// starterOptions are init's flags. The owner has no default: lawgate never
// names a reviewer on the adopter's behalf.
type starterOptions struct {
	root     string
	platform string
	owner    string
	fallback bool
}

func initCommand(arguments []string) error {
	var options starterOptions
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.StringVar(&options.root, "root", ".", "repository root")
	flags.StringVar(&options.platform, "platform", "github", "github or gitlab")
	flags.StringVar(&options.owner, "owner", "", "required CODEOWNER, such as @user or @org/team")
	flags.BoolVar(&options.fallback, "fallback", false, "start CODEOWNERS with '* OWNER', so the owner reviews every file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("init accepts flags only")
	}
	if !inventory.ValidPlatform(options.platform) {
		return fmt.Errorf("unsupported platform %q", options.platform)
	}
	if err := rulechanges.ValidOwner(options.owner); err != nil {
		return err
	}
	root, err := filepath.Abs(options.root)
	if err != nil {
		return err
	}
	options.root = root
	return initialize(options)
}

func initialize(options starterOptions) error {
	owners, ci := starterCI(options.platform)
	names := []string{inventory.Filename, inventory.DefaultBaseline, owners, ci.Name}
	for _, name := range names {
		if err := checkStarterDestination(options.root, name); err != nil {
			return err
		}
	}
	config, err := starterConfig(options.root, options.platform, options.owner)
	if err != nil {
		return err
	}
	block, err := starterOwners(options, config, names)
	if err != nil {
		return err
	}
	configFile := starterFile{Name: inventory.Filename, Data: config}
	baselineFile := starterFile{Name: inventory.DefaultBaseline}
	ownersFile := starterFile{Name: owners, Data: block}
	return writeStarterFiles(options.root, []starterFile{configFile, baselineFile, ownersFile, ci})
}

// starterOwners is the CODEOWNERS content: the optional fallback, then one
// exact entry per rule file the starter configuration makes.
func starterOwners(options starterOptions, config []byte, names []string) ([]byte, error) {
	files, err := rulechanges.StarterRuleFiles(options.root, config, names)
	if err != nil {
		return nil, err
	}
	var block bytes.Buffer
	if options.fallback {
		err = writeFallback(&block, options.platform, options.owner)
	}
	if err != nil {
		return nil, err
	}
	err = rulechanges.WriteOwners(&block, files, options.owner)
	return block.Bytes(), err
}

// starterCI picks the platform's files. The CI reads the owner from the
// base's lawgate.json, so the template needs no owner of its own.
func starterCI(platform string) (string, starterFile) {
	name := ".github/workflows/lawgate.yml"
	owners := ".github/CODEOWNERS"
	content := templates.GitHub
	if platform == "gitlab" {
		name, owners, content = ".gitlab-ci.yml", "CODEOWNERS", templates.GitLab
	}
	return owners, starterFile{Name: name, Data: []byte(content)}
}

func starterConfig(root, platform, owner string) ([]byte, error) {
	modules, err := discoverModules(root)
	if err != nil {
		return nil, err
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no Go modules found under %s", root)
	}
	settings := &inventory.Readability{Baseline: inventory.DefaultBaseline}
	config := inventory.Config{Modules: modules, Platform: platform, Owner: owner, Readability: settings}
	data, err := json.MarshalIndent(config, "", "  ")
	return append(data, '\n'), err
}

// discoverModules lists one starter module per go.mod, using the same
// source walker (and skip rules) as the gate.
func discoverModules(root string) ([]inventory.Module, error) {
	directories, err := inventory.GoModules(root)
	if err != nil {
		return nil, err
	}
	modules := make([]inventory.Module, 0, len(directories))
	for _, directory := range directories {
		modules = append(modules, starterModule(directory))
	}
	return modules, nil
}

func starterModule(directory string) inventory.Module {
	machines := false
	return inventory.Module{
		Path: directory, Language: "go", Laws: path.Join(directory, "LAWS.md"), Machines: &machines,
	}
}

func checkStarterDestination(root, name string) error {
	full := filepath.Join(root, name)
	if _, err := os.Lstat(full); !os.IsNotExist(err) {
		return fmt.Errorf("refusing to overwrite %s", full)
	}
	for parent := filepath.Dir(full); parent != root; parent = filepath.Dir(parent) {
		if err := checkStarterParent(parent); err != nil {
			return err
		}
	}
	return nil
}

func checkStarterParent(parent string) error {
	info, err := os.Lstat(parent)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", parent)
	}
	return nil
}

func writeStarterFiles(root string, files []starterFile) error {
	var created []string
	for _, entry := range files {
		path := filepath.Join(root, entry.Name)
		file, err := createStarterFile(path)
		if err != nil {
			return errors.Join(err, removeStarterFiles(created))
		}
		created = append(created, path)
		_, err = file.Write(entry.Data)
		if err := errors.Join(err, file.Close()); err != nil {
			return errors.Join(err, removeStarterFiles(created))
		}
	}
	return nil
}

func createStarterFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
}

func removeStarterFiles(files []string) error {
	var err error
	for _, path := range files {
		err = errors.Join(err, os.Remove(path))
	}
	return err
}

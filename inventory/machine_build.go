package inventory

import (
	"fmt"
	"strings"
)

// GoMachineBuild describes the source inventory of an opaque Go runner.
type GoMachineBuild struct {
	GOOS       string   `json:"goos,omitempty"`
	GOARCH     string   `json:"goarch,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	CGOEnabled string   `json:"cgo_enabled,omitempty"`
}

func (module Module) checkMachineBuild() error {
	_, directGo := goInvocation(module.MachineCommand)
	customGoRunner := module.Language == "go" && len(module.MachineCommand) != 0
	opaque := customGoRunner && directGo == nil
	switch {
	case opaque && module.MachineBuild == nil:
		return fmt.Errorf("module %q: opaque Go machine command requires machine_build", module.Path)
	case !opaque && module.MachineBuild != nil:
		return fmt.Errorf("module %q: machine_build requires an opaque Go machine command", module.Path)
	case module.MachineBuild != nil:
		return module.MachineBuild.validate(module.Path)
	}
	return nil
}

func (build GoMachineBuild) validate(module string) error {
	switch build.CGOEnabled {
	case "", "0", "1":
		return nil
	default:
		return fmt.Errorf("module %q: machine_build.cgo_enabled must be 0 or 1", module)
	}
}

func (module Module) machineBuildCommand() []string {
	if module.MachineBuild == nil {
		return module.MachineCommand
	}
	build := module.MachineBuild
	var command []string
	command = addBuildOverride(command, "GOOS", build.GOOS)
	command = addBuildOverride(command, "GOARCH", build.GOARCH)
	command = addBuildOverride(command, "CGO_ENABLED", build.CGOEnabled)
	if len(command) != 0 {
		command = append([]string{"env"}, command...)
	}
	command = append(command, "go", "run")
	if len(build.Tags) != 0 {
		command = append(command, "-tags="+strings.Join(build.Tags, ","))
	}
	return command
}

func addBuildOverride(command []string, name, value string) []string {
	if value != "" {
		command = append(command, name+"="+value)
	}
	return command
}

// machineSourceCommand uses the standard runner when machines are enabled.
func (module Module) machineSourceCommand() []string {
	command := module.machineBuildCommand()
	enabled := module.Machines != nil && *module.Machines
	if len(command) == 0 && enabled {
		return defaultMachineCommand()
	}
	return command
}

package environment

import (
	"fmt"
	"strings"
)

// WrapCommand wraps a command with environment activation if needed.
// For Python, this prepends venv activation. For Node, nvm use.
func WrapCommand(env *ProjectEnv, command string) string {
	if env == nil {
		return command
	}

	switch env.Language {
	case "python":
		return wrapPythonCommand(env, command)
	case "javascript":
		return wrapNodeCommand(env, command)
	default:
		return command
	}
}

func wrapPythonCommand(env *ProjectEnv, command string) string {
	if env.VenvPath == "" {
		return command
	}

	activate := fmt.Sprintf("source %s/bin/activate", env.VenvPath)

	// Don't double-wrap
	if strings.Contains(command, "activate") {
		return command
	}

	return activate + " && " + command
}

func wrapNodeCommand(env *ProjectEnv, command string) string {
	if env.Version == "" {
		return command
	}

	// If nvm is available, prepend nvm use
	if strings.Contains(command, "nvm") {
		return command
	}

	return command
}

// SetupCommands returns the commands needed to set up the project environment.
func SetupCommands(env *ProjectEnv) []string {
	if env == nil {
		return nil
	}

	spec, ok := RuntimeSpecs[env.Language]
	if !ok {
		return nil
	}

	var cmds []string

	// Setup command (create venv, etc.)
	if spec.SetupCommand != "" && env.Language == "python" && env.VenvPath == "" {
		cmds = append(cmds, spec.SetupCommand)
	}

	// Install dependencies
	if spec.InstallCommand != "" {
		cmd := spec.InstallCommand
		if env.Language == "python" && env.VenvPath != "" {
			cmd = WrapCommand(env, cmd)
		}
		cmds = append(cmds, cmd)
	}

	return cmds
}

// TestCommand returns the appropriate test command for the project.
func TestCommand(env *ProjectEnv) string {
	if env == nil {
		return ""
	}

	spec, ok := RuntimeSpecs[env.Language]
	if !ok {
		return ""
	}

	cmd := spec.TestCommand
	if env.Language == "python" {
		cmd = WrapCommand(env, cmd)
	}
	return cmd
}

// BuildCommand returns the appropriate build command for the project.
func BuildCommand(env *ProjectEnv) string {
	if env == nil {
		return ""
	}

	spec, ok := RuntimeSpecs[env.Language]
	if !ok {
		return ""
	}

	return spec.BuildCommand
}

// Summary returns a human-readable description of the environment.
func Summary(env *ProjectEnv) string {
	if env == nil {
		return "no project environment detected"
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Language: %s", env.Language))
	if env.Version != "" {
		parts = append(parts, fmt.Sprintf("Version: %s", env.Version))
	}
	parts = append(parts, fmt.Sprintf("Config: %s", env.PackageFile))
	if len(env.Dependencies) > 0 {
		parts = append(parts, fmt.Sprintf("Dependencies: %d", len(env.Dependencies)))
	}
	if env.VenvPath != "" {
		parts = append(parts, fmt.Sprintf("Venv: %s", env.VenvPath))
	}
	return strings.Join(parts, " | ")
}

package environment

import (
	"os"
	"path/filepath"
)

// CheckBestPractices runs language-specific best practice checks against the project.
func CheckBestPractices(env *ProjectEnv, workDir string) []BestPracticeResult {
	switch env.Language {
	case "go":
		return checkGoBestPractices(env, workDir)
	case "python":
		return checkPythonBestPractices(env, workDir)
	case "javascript":
		return checkJSBestPractices(env, workDir)
	case "rust":
		return checkRustBestPractices(env, workDir)
	case "java":
		return checkJavaBestPractices(env, workDir)
	default:
		return checkGeneralBestPractices(env, workDir)
	}
}

func checkGoBestPractices(env *ProjectEnv, workDir string) []BestPracticeResult {
	var results []BestPracticeResult

	// Check go.mod exists
	results = append(results, checkFileExists(workDir, "go.mod", BestPractice{
		Language:    "go",
		Name:        "go_mod_exists",
		Description: "Project should have go.mod for module management",
	}))

	// Check go.sum exists (indicates deps are resolved)
	results = append(results, checkFileExists(workDir, "go.sum", BestPractice{
		Language:    "go",
		Name:        "go_sum_exists",
		Description: "go.sum should exist for dependency verification",
	}))

	// Check .gitignore exists
	results = append(results, checkFileExists(workDir, ".gitignore", BestPractice{
		Language:    "go",
		Name:        "gitignore_exists",
		Description: "Project should have .gitignore",
	}))

	results = append(results, checkGeneralBestPractices(env, workDir)...)
	return results
}

func checkPythonBestPractices(env *ProjectEnv, workDir string) []BestPracticeResult {
	var results []BestPracticeResult

	// Check for virtual environment
	hasVenv := false
	for _, d := range []string{".venv", "venv", "env"} {
		if info, err := os.Stat(filepath.Join(workDir, d)); err == nil && info.IsDir() {
			hasVenv = true
			break
		}
	}
	results = append(results, BestPracticeResult{
		Practice: BestPractice{
			Language:    "python",
			Name:        "virtual_env",
			Description: "Use virtual environment (never install globally)",
		},
		Passed:  hasVenv,
		Message: boolMsg(hasVenv, "virtual environment found", "no virtual environment detected"),
		AutoFix: "python3 -m venv .venv",
	})

	// Check requirements file exists
	hasReqs := false
	for _, f := range []string{"requirements.txt", "pyproject.toml", "Pipfile"} {
		if _, err := os.Stat(filepath.Join(workDir, f)); err == nil {
			hasReqs = true
			break
		}
	}
	results = append(results, BestPracticeResult{
		Practice: BestPractice{
			Language:    "python",
			Name:        "deps_pinned",
			Description: "Dependencies should be pinned in requirements file",
		},
		Passed:  hasReqs,
		Message: boolMsg(hasReqs, "dependency file found", "no requirements.txt or pyproject.toml"),
	})

	// Check for known Python version constraints from deps
	for _, dep := range env.Dependencies {
		if constraint := knownPythonConstraint(dep.Name); constraint != "" {
			results = append(results, BestPracticeResult{
				Practice: BestPractice{
					Language:    "python",
					Name:        "python_version_compat",
					Description: dep.Name + " has Python version constraints",
				},
				Passed:  true, // informational
				Message: constraint,
			})
		}
	}

	results = append(results, checkGeneralBestPractices(env, workDir)...)
	return results
}

// knownPythonConstraint returns known Python version constraints for popular packages.
func knownPythonConstraint(pkg string) string {
	constraints := map[string]string{
		"tensorflow":      "requires Python <=3.12 (3.13+ not supported)",
		"torch":           "requires Python >=3.9, <=3.12",
		"numpy":           "1.x: Python >=3.9; 2.x: Python >=3.10",
		"scipy":           "requires Python >=3.10",
		"scikit-learn":    "requires Python >=3.9",
		"pandas":          "2.x: Python >=3.9",
		"matplotlib":      "requires Python >=3.9",
		"opencv-python":   "requires Python >=3.8",
		"transformers":    "requires Python >=3.8",
	}
	return constraints[pkg]
}

func checkJSBestPractices(env *ProjectEnv, workDir string) []BestPracticeResult {
	var results []BestPracticeResult

	// Check for lockfile
	hasLock := false
	for _, f := range []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml"} {
		if _, err := os.Stat(filepath.Join(workDir, f)); err == nil {
			hasLock = true
			break
		}
	}
	results = append(results, BestPracticeResult{
		Practice: BestPractice{
			Language:    "javascript",
			Name:        "lockfile_exists",
			Description: "Use lockfile for deterministic builds",
		},
		Passed:  hasLock,
		Message: boolMsg(hasLock, "lockfile found", "no lockfile (package-lock.json, yarn.lock, or pnpm-lock.yaml)"),
		AutoFix: "npm install",
	})

	// Check for .nvmrc
	hasNvmrc := false
	if _, err := os.Stat(filepath.Join(workDir, ".nvmrc")); err == nil {
		hasNvmrc = true
	}
	results = append(results, BestPracticeResult{
		Practice: BestPractice{
			Language:    "javascript",
			Name:        "node_version_pinned",
			Description: "Pin Node version via .nvmrc or engines field",
		},
		Passed:  hasNvmrc || env.Version != "",
		Message: boolMsg(hasNvmrc || env.Version != "", "Node version specified", "consider adding .nvmrc or engines in package.json"),
	})

	results = append(results, checkGeneralBestPractices(env, workDir)...)
	return results
}

func checkRustBestPractices(env *ProjectEnv, workDir string) []BestPracticeResult {
	var results []BestPracticeResult

	results = append(results, checkFileExists(workDir, "Cargo.lock", BestPractice{
		Language:    "rust",
		Name:        "cargo_lock",
		Description: "Cargo.lock should be committed for binary projects",
	}))

	results = append(results, checkGeneralBestPractices(env, workDir)...)
	return results
}

func checkJavaBestPractices(env *ProjectEnv, workDir string) []BestPracticeResult {
	var results []BestPracticeResult

	// Check for wrapper
	hasWrapper := false
	for _, f := range []string{"gradlew", "mvnw"} {
		if _, err := os.Stat(filepath.Join(workDir, f)); err == nil {
			hasWrapper = true
			break
		}
	}
	results = append(results, BestPracticeResult{
		Practice: BestPractice{
			Language:    "java",
			Name:        "wrapper_exists",
			Description: "Use Gradle wrapper or Maven wrapper",
		},
		Passed:  hasWrapper,
		Message: boolMsg(hasWrapper, "build wrapper found", "consider adding gradlew or mvnw"),
	})

	results = append(results, checkGeneralBestPractices(env, workDir)...)
	return results
}

func checkGeneralBestPractices(env *ProjectEnv, workDir string) []BestPracticeResult {
	var results []BestPracticeResult

	results = append(results, checkFileExists(workDir, ".gitignore", BestPractice{
		Language:    "general",
		Name:        "gitignore",
		Description: "Project should have .gitignore for build artifacts",
	}))

	return results
}

func checkFileExists(workDir, file string, practice BestPractice) BestPracticeResult {
	_, err := os.Stat(filepath.Join(workDir, file))
	exists := err == nil
	return BestPracticeResult{
		Practice: practice,
		Passed:   exists,
		Message:  boolMsg(exists, file+" found", file+" not found"),
	}
}

func boolMsg(ok bool, pass, fail string) string {
	if ok {
		return pass
	}
	return fail
}

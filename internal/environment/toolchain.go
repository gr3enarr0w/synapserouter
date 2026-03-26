package environment

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveBuildCommands detects the build toolchain for ANY language project
// and returns install, test, build commands. Handles all language variants.
func ResolveBuildCommands(language, workDir string) (install, test, build string) {
	switch language {
	case "java", "kotlin":
		return resolveJavaBuild(workDir) // Kotlin uses same build tools
	case "python":
		return resolvePythonBuild(workDir)
	case "javascript", "typescript":
		return resolveJSBuild(workDir)
	case "r":
		return resolveRBuild(workDir)
	case "cpp", "c":
		return resolveCppBuild(workDir)
	case "swift":
		return "swift build", "swift test", "swift build -c release"
	case "objective-c":
		return resolveCppBuild(workDir) // Often uses Makefiles or Xcode
	case "go":
		return "go mod download", "go test -race ./...", "go build ./..."
	case "rust":
		return "cargo build", "cargo test", "cargo build --release"
	case "ruby":
		return resolveRubyBuild(workDir)
	case "scala":
		return resolveScalaBuild(workDir)
	case "csharp", "fsharp":
		return "dotnet restore", "dotnet test", "dotnet build"
	case "dart", "flutter":
		if fileExists(filepath.Join(workDir, "pubspec.yaml")) {
			if commandExists("flutter") {
				return "flutter pub get", "flutter test", "flutter build"
			}
			return "dart pub get", "dart test", "dart compile exe"
		}
		return "dart pub get", "dart test", "dart compile exe"
	case "elixir":
		return "mix deps.get", "mix test", "mix compile"
	case "haskell":
		if fileExists(filepath.Join(workDir, "stack.yaml")) {
			return "stack setup && stack build", "stack test", "stack build"
		}
		return "cabal update && cabal build", "cabal test", "cabal build"
	case "lua":
		return "luarocks install --deps-only", "busted", ""
	case "perl":
		return "cpanm --installdeps .", "prove -r t/", ""
	case "php":
		return "composer install", "vendor/bin/phpunit", ""
	default:
		spec, ok := RuntimeSpecs[language]
		if ok {
			return spec.InstallCommand, spec.TestCommand, spec.BuildCommand
		}
		return "", "", ""
	}
}

func resolveRBuild(workDir string) (install, test, build string) {
	// Check for renv (modern R project management)
	if fileExists(filepath.Join(workDir, "renv.lock")) {
		return "Rscript -e 'renv::restore()'", "Rscript -e 'testthat::test_dir(\"tests\")'", ""
	}
	// Check for DESCRIPTION (R package)
	if fileExists(filepath.Join(workDir, "DESCRIPTION")) {
		return "Rscript -e 'devtools::install_deps()'", "Rscript -e 'devtools::test()'", "R CMD build ."
	}
	return "", "Rscript -e 'testthat::test_dir(\"tests\")'", ""
}

func resolveCppBuild(workDir string) (install, test, build string) {
	// CMake (most common)
	if fileExists(filepath.Join(workDir, "CMakeLists.txt")) {
		return "cmake -B build && cmake --build build", "cd build && ctest", "cmake --build build --config Release"
	}
	// Meson
	if fileExists(filepath.Join(workDir, "meson.build")) {
		return "meson setup build && ninja -C build", "ninja -C build test", "ninja -C build"
	}
	// Makefile
	if fileExists(filepath.Join(workDir, "Makefile")) {
		return "make", "make test", "make"
	}
	// Bazel
	if fileExists(filepath.Join(workDir, "BUILD")) || fileExists(filepath.Join(workDir, "WORKSPACE")) {
		return "bazel build //...", "bazel test //...", "bazel build //..."
	}
	// Default: plain make
	return "make", "make test", "make"
}

func resolveRubyBuild(workDir string) (install, test, build string) {
	if fileExists(filepath.Join(workDir, "Gemfile")) {
		return "bundle install", "bundle exec rake test", "bundle exec rake build"
	}
	return "gem install bundler && bundle install", "ruby -Itest test/", ""
}

func resolveScalaBuild(workDir string) (install, test, build string) {
	if fileExists(filepath.Join(workDir, "build.sbt")) {
		return "sbt compile", "sbt test", "sbt assembly"
	}
	// Falls back to Gradle/Maven (common for Scala)
	return resolveJavaBuild(workDir)
}

func resolvePythonBuild(workDir string) (install, test, build string) {
	// Check for conda/mamba environment
	if fileExists(filepath.Join(workDir, "environment.yml")) || fileExists(filepath.Join(workDir, "environment.yaml")) {
		envFile := "environment.yml"
		if fileExists(filepath.Join(workDir, "environment.yaml")) {
			envFile = "environment.yaml"
		}
		if commandExists("mamba") {
			return "mamba env update -f " + envFile, "python -m pytest", ""
		}
		if commandExists("conda") {
			return "conda env update -f " + envFile, "python -m pytest", ""
		}
		return "conda env update -f " + envFile, "python -m pytest", ""
	}
	// Check for uv (fastest, modern)
	if fileExists(filepath.Join(workDir, "uv.lock")) || fileExists(filepath.Join(workDir, "uv.toml")) {
		if commandExists("uv") {
			return "uv sync", "uv run pytest", "uv build"
		}
		return "pip install uv && uv sync", "uv run pytest", "uv build"
	}
	// Check for poetry
	if fileExists(filepath.Join(workDir, "pyproject.toml")) {
		content, _ := os.ReadFile(filepath.Join(workDir, "pyproject.toml"))
		contentStr := string(content)
		if strings.Contains(contentStr, "[tool.poetry]") {
			if commandExists("poetry") {
				return "poetry install", "poetry run pytest", "poetry build"
			}
			return "pip install poetry && poetry install", "poetry run pytest", "poetry build"
		}
		// Check for PDM
		if strings.Contains(contentStr, "[tool.pdm]") {
			if commandExists("pdm") {
				return "pdm install", "pdm run pytest", "pdm build"
			}
			return "pip install pdm && pdm install", "pdm run pytest", "pdm build"
		}
		// Check for hatch
		if strings.Contains(contentStr, "[tool.hatch]") {
			if commandExists("hatch") {
				return "hatch env create", "hatch run test", "hatch build"
			}
			return "pip install hatch && hatch env create", "hatch run test", "hatch build"
		}
		// Generic pyproject.toml — use pip or uv
		if commandExists("uv") {
			return "uv pip install -e .", "uv run pytest", "uv build"
		}
		return "pip install -e .", "python -m pytest", "python -m build"
	}
	// Check for Pipfile (pipenv)
	if fileExists(filepath.Join(workDir, "Pipfile")) {
		if commandExists("pipenv") {
			return "pipenv install --dev", "pipenv run pytest", ""
		}
		return "pip install pipenv && pipenv install --dev", "pipenv run pytest", ""
	}
	// Check for setup.py (legacy)
	if fileExists(filepath.Join(workDir, "setup.py")) {
		return "pip install -e '.[dev]'", "python -m pytest", "python setup.py sdist bdist_wheel"
	}
	// Default: requirements.txt + pip
	if fileExists(filepath.Join(workDir, "requirements.txt")) {
		return "pip install -r requirements.txt", "python -m pytest", ""
	}
	return "pip install -e .", "python -m pytest", ""
}

func resolveJSBuild(workDir string) (install, test, build string) {
	// Check for pnpm
	if fileExists(filepath.Join(workDir, "pnpm-lock.yaml")) {
		if commandExists("pnpm") {
			return "pnpm install", "pnpm test", "pnpm run build"
		}
		return "npm install -g pnpm && pnpm install", "pnpm test", "pnpm run build"
	}
	// Check for yarn
	if fileExists(filepath.Join(workDir, "yarn.lock")) {
		if commandExists("yarn") {
			return "yarn install", "yarn test", "yarn build"
		}
		return "npm install -g yarn && yarn install", "yarn test", "yarn build"
	}
	// Check for bun
	if fileExists(filepath.Join(workDir, "bun.lockb")) {
		if commandExists("bun") {
			return "bun install", "bun test", "bun run build"
		}
		return "curl -fsSL https://bun.sh/install | bash && bun install", "bun test", "bun run build"
	}
	// Default npm
	return "npm install", "npm test", "npm run build"
}

// resolveJavaBuild detects whether a Java project uses Maven or Gradle
// and returns the appropriate commands. Prefers wrapper scripts (mvnw/gradlew)
// over system-installed tools.
func resolveJavaBuild(workDir string) (install, test, build string) {
	// Check for Maven wrapper first (most portable)
	if fileExists(filepath.Join(workDir, "mvnw")) {
		return "./mvnw compile", "./mvnw test", "./mvnw package"
	}
	// Check for Gradle wrapper
	if fileExists(filepath.Join(workDir, "gradlew")) {
		return "./gradlew build", "./gradlew test", "./gradlew build"
	}
	// Check for pom.xml (Maven project without wrapper)
	if fileExists(filepath.Join(workDir, "pom.xml")) {
		if commandExists("mvn") {
			return "mvn compile", "mvn test", "mvn package"
		}
		// Maven not installed — return install instructions as setup command
		return "mvn compile", "mvn test", "mvn package"
	}
	// Check for build.gradle (Gradle project without wrapper)
	if fileExists(filepath.Join(workDir, "build.gradle")) || fileExists(filepath.Join(workDir, "build.gradle.kts")) {
		if commandExists("gradle") {
			return "gradle build", "gradle test", "gradle build"
		}
		return "gradle build", "gradle test", "gradle build"
	}
	// Default to Maven (most common for Spring Boot)
	return "mvn compile", "mvn test", "mvn package"
}

// MissingTools checks which required tools are missing for a detected project.
// Returns a list of tool names that need to be installed.
func MissingTools(env *ProjectEnv, workDir string) []string {
	if env == nil {
		return nil
	}

	var missing []string
	switch env.Language {
	case "java", "kotlin", "scala":
		if !commandExists("java") && !commandExists("javac") {
			missing = append(missing, "java")
		}
		hasMvnw := fileExists(filepath.Join(workDir, "mvnw"))
		hasGradlew := fileExists(filepath.Join(workDir, "gradlew"))
		hasPom := fileExists(filepath.Join(workDir, "pom.xml"))
		hasGradle := fileExists(filepath.Join(workDir, "build.gradle")) || fileExists(filepath.Join(workDir, "build.gradle.kts"))
		if hasPom && !hasMvnw && !commandExists("mvn") {
			missing = append(missing, "maven")
		}
		if hasGradle && !hasGradlew && !commandExists("gradle") {
			missing = append(missing, "gradle")
		}
		if env.Language == "scala" && !commandExists("sbt") {
			missing = append(missing, "sbt")
		}
	case "go":
		if !commandExists("go") {
			missing = append(missing, "go")
		}
	case "python":
		if !commandExists("python3") && !commandExists("python") {
			missing = append(missing, "python3")
		}
	case "javascript", "typescript":
		if !commandExists("node") {
			missing = append(missing, "node")
		}
		if !commandExists("npm") && !commandExists("yarn") && !commandExists("pnpm") {
			missing = append(missing, "npm")
		}
	case "r":
		if !commandExists("Rscript") && !commandExists("R") {
			missing = append(missing, "r")
		}
	case "cpp", "c", "objective-c":
		if !commandExists("gcc") && !commandExists("g++") && !commandExists("clang") {
			missing = append(missing, "gcc")
		}
		if fileExists(filepath.Join(workDir, "CMakeLists.txt")) && !commandExists("cmake") {
			missing = append(missing, "cmake")
		}
	case "swift":
		if !commandExists("swift") {
			missing = append(missing, "swift")
		}
	case "csharp", "fsharp":
		if !commandExists("dotnet") {
			missing = append(missing, "dotnet")
		}
	case "dart", "flutter":
		if !commandExists("dart") && !commandExists("flutter") {
			missing = append(missing, "dart")
		}
	case "elixir":
		if !commandExists("mix") && !commandExists("elixir") {
			missing = append(missing, "elixir")
		}
	case "haskell":
		if !commandExists("ghc") && !commandExists("stack") && !commandExists("cabal") {
			missing = append(missing, "haskell")
		}
	case "php":
		if !commandExists("php") {
			missing = append(missing, "php")
		}
		if !commandExists("composer") {
			missing = append(missing, "composer")
		}
	case "lua":
		if !commandExists("lua") {
			missing = append(missing, "lua")
		}
	case "perl":
		if !commandExists("perl") {
			missing = append(missing, "perl")
		}
	case "rust":
		if !commandExists("cargo") {
			missing = append(missing, "rust")
		}
	case "ruby":
		if !commandExists("ruby") {
			missing = append(missing, "ruby")
		}
	}
	return missing
}

// InstallCommand returns the shell command to install a missing tool.
// Supports Homebrew (macOS) and common Linux package managers.
func InstallCommand(tool string) string {
	if runtime.GOOS == "darwin" {
		return installCommandBrew(tool)
	}
	return installCommandLinux(tool)
}

func installCommandBrew(tool string) string {
	switch tool {
	case "maven":
		return "brew install maven"
	case "gradle":
		return "brew install gradle"
	case "java":
		return "brew install openjdk"
	case "go":
		return "brew install go"
	case "python3":
		return "brew install python3"
	case "node", "npm":
		return "brew install node"
	case "rust":
		return "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y"
	case "ruby":
		return "brew install ruby"
	case "r":
		return "brew install r"
	case "cmake":
		return "brew install cmake"
	case "gcc":
		return "brew install gcc"
	case "swift":
		return "# Swift comes with Xcode: xcode-select --install"
	case "dotnet":
		return "brew install --cask dotnet-sdk"
	case "dart":
		return "brew install dart"
	case "flutter":
		return "brew install --cask flutter"
	case "elixir":
		return "brew install elixir"
	case "haskell":
		return "brew install ghcup && ghcup install ghc"
	case "sbt":
		return "brew install sbt"
	case "php":
		return "brew install php"
	case "composer":
		return "brew install composer"
	case "lua":
		return "brew install lua && brew install luarocks"
	case "perl":
		return "brew install perl"
	case "conda":
		return "brew install --cask miniconda"
	case "poetry":
		return "pip install poetry"
	case "uv":
		return "pip install uv"
	default:
		return fmt.Sprintf("brew install %s", tool)
	}
}

func installCommandLinux(tool string) string {
	switch tool {
	case "maven":
		return "sudo apt-get install -y maven || sudo yum install -y maven"
	case "gradle":
		return "sudo apt-get install -y gradle || sudo yum install -y gradle"
	case "java":
		return "sudo apt-get install -y default-jdk || sudo yum install -y java-17-openjdk-devel"
	default:
		return fmt.Sprintf("# Install %s manually", tool)
	}
}

// EnsureToolchain checks for missing tools and returns install commands.
// The agent should run these before starting the project.
func EnsureToolchain(env *ProjectEnv, workDir string) []string {
	missing := MissingTools(env, workDir)
	if len(missing) == 0 {
		return nil
	}

	var cmds []string
	for _, tool := range missing {
		cmds = append(cmds, InstallCommand(tool))
	}
	return cmds
}

// GenerateMavenWrapper creates a Maven wrapper in the project directory
// so future builds don't require system Maven. Requires Maven to be installed.
func GenerateMavenWrapper(workDir string) error {
	if !commandExists("mvn") {
		return fmt.Errorf("mvn not installed — install with: %s", InstallCommand("maven"))
	}
	cmd := exec.Command("mvn", "wrapper:wrapper")
	cmd.Dir = workDir
	return cmd.Run()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ResolveJavaSpec returns a RuntimeSpec with commands resolved for the actual
// build tool in the project directory.
func ResolveJavaSpec(workDir string) RuntimeSpec {
	install, test, build := resolveJavaBuild(workDir)
	return RuntimeSpec{
		Language:       "java",
		StableVersions: []string{"21", "17", "11"},
		InstallCommand: install,
		TestCommand:    test,
		BuildCommand:   build,
	}
}

// ResolvedSpec returns the RuntimeSpec for a language, resolving dynamic
// specs (like Java) based on the working directory.
func ResolvedSpec(language, workDir string) (RuntimeSpec, bool) {
	if language == "java" && workDir != "" {
		return ResolveJavaSpec(workDir), true
	}
	spec, ok := RuntimeSpecs[language]
	return spec, ok
}

// SetupInstructions returns human-readable instructions for missing tools.
func SetupInstructions(env *ProjectEnv, workDir string) string {
	missing := MissingTools(env, workDir)
	if len(missing) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Missing tools detected:\n")
	for _, tool := range missing {
		b.WriteString(fmt.Sprintf("  - %s: install with `%s`\n", tool, InstallCommand(tool)))
	}
	return b.String()
}

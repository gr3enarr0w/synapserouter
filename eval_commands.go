package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/app"
	"github.com/gr3enarr0w/synapserouter/internal/eval"
)

func cmdEval(args []string) {
	if len(args) == 0 {
		printEvalUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		cmdEvalImport(args[1:])
	case "import-all":
		cmdEvalImportAll(args[1:])
	case "exercises":
		cmdEvalExercises(args[1:])
	case "run":
		cmdEvalRun(args[1:])
	case "results":
		cmdEvalResults(args[1:])
	case "compare":
		cmdEvalCompare(args[1:])
	case "help", "--help":
		printEvalUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown eval subcommand: %s\n", args[0])
		printEvalUsage()
		os.Exit(1)
	}
}

func printEvalUsage() {
	fmt.Println(`synroute eval — Multi-language eval framework

Usage:
  synroute eval <command> [flags]

Commands:
  import      Import exercises from benchmark repos
  import-all  Clone and import all benchmark sources
  exercises   List imported exercises
  run         Run an evaluation
  results     Show results from a run
  compare     Compare two runs

Sources:
  polyglot      Aider polyglot-benchmark (~225 exercises)
  roocode       Roo-Code-Evals (~170 exercises)
  exercism      Exercism tracks (100-159 per language, requires --language)
  multiple      MultiPL-E HumanEval translations (~984 across 6 languages)
  evalplus      EvalPlus HumanEval+/MBPP+ (Python only, ~563 exercises)
  codecontests  Google CodeContests (stdin/stdout problems, use --count to limit)
  ds1000        DS-1000 data science problems (~1000, Python)
  birdsql       BIRD-SQL Mini-Dev text-to-SQL (~500 exercises)
  dare-bench    DARE-bench ML modeling (~162 tasks, metric-based)
  writingbench  WritingBench business writing (~1000, LLM-judge)
  pptarena      PPTArena slide editing (~100 tasks, VLM-judge deferred)

Examples:
  synroute eval import --source polyglot --path ~/polyglot-benchmark
  synroute eval import --source exercism --path ~/exercism-go --language go
  synroute eval import --source multiple --path ~/MultiPL-E
  synroute eval import --source evalplus --path ~/evalplus
  synroute eval import --source codecontests --path ~/code_contests --count 500
  synroute eval import --source ds1000 --path benchmarks/ds1000
  synroute eval import --source birdsql --path benchmarks/birdsql
  synroute eval import --source dare-bench --path benchmarks/dare-bench
  synroute eval import --source writingbench --path benchmarks/writingbench
  synroute eval import --source pptarena --path benchmarks/pptarena
  synroute eval import-all --dir ~/eval-benchmarks
  synroute eval exercises --language go --json
  synroute eval run --language go --count 10 --two-pass
  synroute eval results --run-id eval-abc123 --json
  synroute eval compare --run-a eval-abc --run-b eval-def`)
}

func cmdEvalImport(args []string) {
	fs := flag.NewFlagSet("eval import", flag.ExitOnError)
	source := fs.String("source", "", "Source: polyglot, roocode, exercism, multiple, evalplus, codecontests, ds1000, birdsql, dare-bench, writingbench, pptarena")
	path := fs.String("path", "", "Path to cloned benchmark repo")
	language := fs.String("language", "", "Language (required for exercism)")
	count := fs.Int("count", 0, "Max exercises to import (codecontests only, 0=all)")
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	if *source == "" || *path == "" {
		fmt.Fprintln(os.Stderr, "Usage: synroute eval import --source <polyglot|roocode|exercism|multiple|evalplus|codecontests|ds1000|birdsql|dare-bench|writingbench|pptarena> --path <repo-path>")
		os.Exit(1)
	}

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	store := eval.NewStore(ac.DB)

	var result *eval.ImportResult
	switch *source {
	case "polyglot":
		result, err = eval.ImportPolyglot(store, *path)
	case "roocode":
		result, err = eval.ImportRooCode(store, *path)
	case "exercism":
		if *language == "" {
			fmt.Fprintln(os.Stderr, "Error: --language is required for exercism source (one repo per language)")
			os.Exit(1)
		}
		result, err = eval.ImportExercism(store, *path, *language)
	case "multiple":
		result, err = eval.ImportMultiPLE(store, *path)
	case "evalplus":
		result, err = eval.ImportEvalPlus(store, *path)
	case "codecontests":
		result, err = eval.ImportCodeContests(store, *path, *count)
	case "ds1000":
		result, err = eval.ImportDS1000(store, *path)
	case "birdsql":
		result, err = eval.ImportBIRDSQL(store, *path)
	case "dare-bench":
		result, err = eval.ImportDAREBench(store, *path)
	case "writingbench":
		result, err = eval.ImportWritingBench(store, *path)
	case "pptarena":
		result, err = eval.ImportPPTArena(store, *path)
	default:
		fmt.Fprintf(os.Stderr, "Unknown source: %s (use polyglot, roocode, exercism, multiple, evalplus, codecontests, ds1000, birdsql, dare-bench, writingbench, or pptarena)\n", *source)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Import error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return
	}

	fmt.Printf("Import complete (%s):\n", result.Suite)
	fmt.Printf("  Imported: %d\n", result.Imported)
	fmt.Printf("  Skipped:  %d\n", result.Skipped)
	fmt.Printf("  Errors:   %d\n", result.Errors)
}

func cmdEvalImportAll(args []string) {
	fs := flag.NewFlagSet("eval import-all", flag.ExitOnError)
	dir := fs.String("dir", "", "Directory to clone repos into")
	codecontestsCount := fs.Int("codecontests-count", 500, "Max CodeContests exercises to import")
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "Usage: synroute eval import-all --dir <directory>")
		os.Exit(1)
	}

	if err := os.MkdirAll(*dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	store := eval.NewStore(ac.DB)

	type repoSpec struct {
		name string
		url  string
	}

	repos := []repoSpec{
		{"polyglot-benchmark", "https://github.com/Aider-AI/polyglot-benchmark"},
		{"Roo-Code-Evals", "https://github.com/RooVetGit/Roo-Code-Evals"},
		{"exercism-go", "https://github.com/exercism/go"},
		{"exercism-python", "https://github.com/exercism/python"},
		{"exercism-javascript", "https://github.com/exercism/javascript"},
		{"exercism-java", "https://github.com/exercism/java"},
		{"exercism-rust", "https://github.com/exercism/rust"},
		{"exercism-cpp", "https://github.com/exercism/cpp"},
		{"MultiPL-E", "https://github.com/nuprl/MultiPL-E"},
		{"evalplus", "https://github.com/evalplus/evalplus"},
		{"code_contests", "https://github.com/google-deepmind/code_contests"},
		{"DS-1000", "https://github.com/xlang-ai/DS-1000"},
		{"dare-bench", "https://github.com/Snowflake-Labs/dare-bench"},
		{"mini_dev", "https://github.com/bird-bench/mini_dev"},
		{"WritingBench", "https://github.com/X-PLUG/WritingBench"},
		{"PPTArena", "https://github.com/michaelofengend/PPTArena"},
	}

	// Clone repos
	for _, repo := range repos {
		repoPath := filepath.Join(*dir, repo.name)
		if _, err := os.Stat(repoPath); err == nil {
			fmt.Printf("  %s already exists, skipping clone\n", repo.name)
			continue
		}
		fmt.Printf("  Cloning %s...\n", repo.name)
		cmd := exec.Command("git", "clone", "--depth", "1", repo.url, repoPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to clone %s: %v\n", repo.name, err)
		}
	}

	type importJob struct {
		suite    string
		language string
		path     string
	}

	exercismLangs := map[string]string{
		"go": "exercism-go", "python": "exercism-python", "javascript": "exercism-javascript",
		"java": "exercism-java", "rust": "exercism-rust", "cpp": "exercism-cpp",
	}

	var allResults []eval.ImportResult

	// Import polyglot
	polyPath := filepath.Join(*dir, "polyglot-benchmark")
	if _, err := os.Stat(polyPath); err == nil {
		fmt.Printf("\nImporting polyglot...\n")
		if r, err := eval.ImportPolyglot(store, polyPath); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import roocode
	rooPath := filepath.Join(*dir, "Roo-Code-Evals")
	if _, err := os.Stat(rooPath); err == nil {
		fmt.Printf("Importing roocode...\n")
		if r, err := eval.ImportRooCode(store, rooPath); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import exercism tracks
	for lang, repoName := range exercismLangs {
		repoPath := filepath.Join(*dir, repoName)
		if _, err := os.Stat(repoPath); err != nil {
			continue
		}
		fmt.Printf("Importing exercism/%s...\n", lang)
		if r, err := eval.ImportExercism(store, repoPath, lang); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import MultiPL-E
	multiplePath := filepath.Join(*dir, "MultiPL-E")
	if _, err := os.Stat(multiplePath); err == nil {
		fmt.Printf("Importing multiple...\n")
		if r, err := eval.ImportMultiPLE(store, multiplePath); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import EvalPlus
	evalPlusPath := filepath.Join(*dir, "evalplus")
	if _, err := os.Stat(evalPlusPath); err == nil {
		fmt.Printf("Importing evalplus...\n")
		if r, err := eval.ImportEvalPlus(store, evalPlusPath); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import CodeContests
	ccPath := filepath.Join(*dir, "code_contests")
	if _, err := os.Stat(ccPath); err == nil {
		fmt.Printf("Importing codecontests (max %d)...\n", *codecontestsCount)
		if r, err := eval.ImportCodeContests(store, ccPath, *codecontestsCount); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import DS-1000
	ds1000Path := filepath.Join(*dir, "DS-1000")
	if _, err := os.Stat(ds1000Path); err == nil {
		fmt.Printf("Importing ds1000...\n")
		if r, err := eval.ImportDS1000(store, ds1000Path); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import DARE-bench
	darePath := filepath.Join(*dir, "dare-bench")
	if _, err := os.Stat(darePath); err == nil {
		fmt.Printf("Importing dare-bench...\n")
		if r, err := eval.ImportDAREBench(store, darePath); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import BIRD-SQL
	birdPath := filepath.Join(*dir, "mini_dev")
	if _, err := os.Stat(birdPath); err == nil {
		fmt.Printf("Importing birdsql...\n")
		if r, err := eval.ImportBIRDSQL(store, birdPath); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import WritingBench
	wbPath := filepath.Join(*dir, "WritingBench")
	if _, err := os.Stat(wbPath); err == nil {
		fmt.Printf("Importing writingbench...\n")
		if r, err := eval.ImportWritingBench(store, wbPath); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Import PPTArena
	pptPath := filepath.Join(*dir, "PPTArena")
	if _, err := os.Stat(pptPath); err == nil {
		fmt.Printf("Importing pptarena...\n")
		if r, err := eval.ImportPPTArena(store, pptPath); err == nil {
			allResults = append(allResults, *r)
			fmt.Printf("  Imported: %d, Skipped: %d\n", r.Imported, r.Skipped)
		} else {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		}
	}

	// Summary
	totalImported, totalSkipped, totalErrors := 0, 0, 0
	for _, r := range allResults {
		totalImported += r.Imported
		totalSkipped += r.Skipped
		totalErrors += r.Errors
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]interface{}{
			"total_imported": totalImported,
			"total_skipped":  totalSkipped,
			"total_errors":   totalErrors,
			"sources":        allResults,
		})
		return
	}

	fmt.Printf("\nImport-all complete:\n")
	fmt.Printf("  Total imported: %d\n", totalImported)
	fmt.Printf("  Total skipped:  %d\n", totalSkipped)
	fmt.Printf("  Total errors:   %d\n", totalErrors)
}

func cmdEvalExercises(args []string) {
	fs := flag.NewFlagSet("eval exercises", flag.ExitOnError)
	language := fs.String("language", "", "Filter by language")
	suite := fs.String("suite", "", "Filter by suite")
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	store := eval.NewStore(ac.DB)
	exercises, err := store.ListExercises(*suite, *language)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing exercises: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(exercises)
		return
	}

	fmt.Printf("Exercises (%d):\n\n", len(exercises))
	fmt.Printf("%-40s %-12s %-10s\n", "ID", "LANGUAGE", "SUITE")
	fmt.Println(strings.Repeat("-", 65))
	for _, ex := range exercises {
		fmt.Printf("%-40s %-12s %-10s\n", ex.ID, ex.Language, ex.Suite)
	}
}

func cmdEvalRun(args []string) {
	fs := flag.NewFlagSet("eval run", flag.ExitOnError)
	language := fs.String("language", "", "Languages (comma-separated)")
	suite := fs.String("suite", "", "Filter by suite")
	provider := fs.String("provider", "", "Specific provider (direct mode)")
	model := fs.String("model", "", "Specific model")
	mode := fs.String("mode", "direct", "Mode: direct or routing")
	count := fs.Int("count", 0, "Total exercise cap (0 = no cap)")
	perSuite := fs.Int("per-suite", 40, "Exercises per suite (0 = all, default 40 for pipeline validation)")
	seed := fs.Int64("seed", 0, "Random seed for reproducibility")
	twoPass := fs.Bool("two-pass", false, "Enable two-pass with error feedback")
	agentMode := fs.Bool("agent", false, "Agent mode: iterative test→fix loop with test file context")
	maxTurns := fs.Int("max-turns", 5, "Max fix iterations in agent mode")
	timeout := fs.Int("timeout", 120, "Per-exercise Docker timeout in seconds")
	concurrency := fs.Int("concurrency", 4, "Parallel exercises (1=sequential, max 10)")
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	if *provider != "" && *mode == "direct" {
		// Keep direct mode when provider is specified
	} else if *provider == "" && *mode == "direct" {
		*mode = "routing"
	}

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()
	ac.InitFull()

	store := eval.NewStore(ac.DB)

	config := eval.EvalRunConfig{
		Suite:         *suite,
		Provider:      *provider,
		Model:         *model,
		Mode:          *mode,
		Count:         *count,
		CountPerSuite: *perSuite,
		Seed:          *seed,
		TwoPass:       *twoPass,
		AgentMode:     *agentMode,
		MaxTurns:      *maxTurns,
		Timeout:       *timeout,
		Concurrency:   *concurrency,
	}

	if *language != "" {
		config.Languages = strings.Split(*language, ",")
	}

	if !eval.IsDockerAvailable() {
		fmt.Fprintln(os.Stderr, "Warning: Docker is not available. Using native test execution (go test, pytest, etc.)")
	}

	runner := eval.NewRunner(store, ac.ProxyRouter, ac.Providers)

	fmt.Printf("Starting eval run (mode=%s", config.Mode)
	if config.Provider != "" {
		fmt.Printf(", provider=%s", config.Provider)
	}
	if len(config.Languages) > 0 {
		fmt.Printf(", languages=%s", strings.Join(config.Languages, ","))
	}
	if config.CountPerSuite > 0 {
		fmt.Printf(", per-suite=%d", config.CountPerSuite)
	}
	if config.Count > 0 {
		fmt.Printf(", count=%d", config.Count)
	}
	if config.TwoPass {
		fmt.Print(", two-pass")
	}
	fmt.Println(")")

	start := time.Now()
	run, err := runner.Run(context.Background(), config)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Eval run failed: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(run)
		return
	}

	fmt.Printf("\nRun %s completed in %s\n", run.ID, elapsed.Round(time.Second))
	if run.Summary != nil {
		s := run.Summary
		fmt.Printf("\nResults:\n")
		fmt.Printf("  Exercises: %d\n", s.TotalExercises)
		fmt.Printf("  Pass@1:    %.1f%%\n", s.Pass1Rate*100)
		fmt.Printf("  Pass@2:    %.1f%%\n", s.Pass2Rate*100)
		fmt.Printf("  Avg Latency: %dms\n", s.AvgLatencyMs)
		fmt.Printf("  Total Tokens: %d\n", s.TotalTokens)
		fmt.Printf("  Fallback Rate: %.1f%%\n", s.FallbackRate*100)
		if s.MetricResults > 0 {
			fmt.Printf("  Avg Metric Score: %.2f (%d scored)\n", s.AvgMetricScore, s.MetricResults)
		}

		if len(s.ByProvider) > 0 {
			fmt.Printf("\nBy Provider:\n")
			for name, ps := range s.ByProvider {
				fmt.Printf("  %-20s pass1=%.0f%% pass2=%.0f%% (%d exercises)\n",
					name, ps.Pass1Rate*100, ps.Pass2Rate*100, ps.Total)
			}
		}
		if len(s.ByLanguage) > 0 {
			fmt.Printf("\nBy Language:\n")
			for name, ls := range s.ByLanguage {
				fmt.Printf("  %-12s pass1=%.0f%% pass2=%.0f%% (%d exercises)\n",
					name, ls.Pass1Rate*100, ls.Pass2Rate*100, ls.Total)
			}
		}
	}
}

func cmdEvalResults(args []string) {
	fs := flag.NewFlagSet("eval results", flag.ExitOnError)
	runID := fs.String("run-id", "", "Run ID (default: most recent)")
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	store := eval.NewStore(ac.DB)

	// If no run ID, get most recent
	if *runID == "" {
		runs, err := store.ListRuns(1)
		if err != nil || len(runs) == 0 {
			fmt.Fprintln(os.Stderr, "No eval runs found")
			os.Exit(1)
		}
		*runID = runs[0].ID
	}

	run, err := store.GetRun(*runID)
	if err != nil || run == nil {
		fmt.Fprintf(os.Stderr, "Run not found: %s\n", *runID)
		os.Exit(1)
	}

	results, err := store.GetResultsByRun(*runID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting results: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]interface{}{
			"run":     run,
			"results": results,
		})
		return
	}

	fmt.Printf("Run: %s (status: %s)\n\n", run.ID, run.Status)
	fmt.Printf("%-40s %-15s %-6s %-6s %-8s\n", "EXERCISE", "PROVIDER", "PASS1", "PASS2", "LATENCY")
	fmt.Println(strings.Repeat("-", 80))
	for _, r := range results {
		pass1 := "FAIL"
		if r.Pass1 {
			pass1 = "PASS"
		}
		pass2 := "-"
		if r.Pass2 {
			pass2 = "PASS"
		} else if r.GeneratedCode2 != "" {
			pass2 = "FAIL"
		}
		fmt.Printf("%-40s %-15s %-6s %-6s %dms\n",
			r.ExerciseID, r.Provider, pass1, pass2, r.LatencyMs)
	}

	if run.Summary != nil {
		fmt.Printf("\nSummary: pass@1=%.1f%% pass@2=%.1f%% avg=%dms tokens=%d\n",
			run.Summary.Pass1Rate*100, run.Summary.Pass2Rate*100,
			run.Summary.AvgLatencyMs, run.Summary.TotalTokens)
	}
}

func cmdEvalCompare(args []string) {
	fs := flag.NewFlagSet("eval compare", flag.ExitOnError)
	runA := fs.String("run-a", "", "First run ID")
	runB := fs.String("run-b", "", "Second run ID")
	jsonOut := fs.Bool("json", false, "Output JSON")
	fs.Parse(args)

	if *runA == "" || *runB == "" {
		fmt.Fprintln(os.Stderr, "Usage: synroute eval compare --run-a <id> --run-b <id>")
		os.Exit(1)
	}

	ac, err := app.InitLight(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}
	defer ac.Close()

	store := eval.NewStore(ac.DB)

	a, err := store.GetRun(*runA)
	if err != nil || a == nil {
		fmt.Fprintf(os.Stderr, "Run A not found: %s\n", *runA)
		os.Exit(1)
	}
	b, err := store.GetRun(*runB)
	if err != nil || b == nil {
		fmt.Fprintf(os.Stderr, "Run B not found: %s\n", *runB)
		os.Exit(1)
	}

	comp := eval.CompareRuns(a, b)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(comp)
		return
	}

	fmt.Printf("Comparison: %s vs %s\n\n", comp.RunA, comp.RunB)
	fmt.Printf("  Pass@1:    %+.1f%%\n", comp.Pass1Delta*100)
	fmt.Printf("  Pass@2:    %+.1f%%\n", comp.Pass2Delta*100)
	fmt.Printf("  Latency:   %+dms\n", comp.LatencyDelta)
	fmt.Printf("  Tokens:    %+d\n", comp.TokensDelta)
	fmt.Printf("  Fallback:  %+.1f%%\n", comp.FallbackDelta*100)
}

package eval

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Store handles persistence for the eval framework.
type Store struct {
	db *sql.DB
	mu sync.Mutex // serialize writes for concurrent eval runs
}

// NewStore creates a Store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// UpsertExercise inserts or replaces an exercise.
func (s *Store) UpsertExercise(ex Exercise) error {
	evalMode := ex.EvalMode
	if evalMode == "" {
		evalMode = "docker-test"
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO eval_exercises
			(id, suite, language, slug, instructions, stub, test_file, test_command, docker_image, eval_mode, reference_code, criteria, imported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ex.ID, ex.Suite, ex.Language, ex.Slug, ex.Instructions, ex.Stub,
		ex.TestFile, ex.TestCommand, ex.DockerImage, evalMode, ex.ReferenceCode, ex.Criteria, time.Now(),
	)
	return err
}

// GetExercise retrieves an exercise by ID.
func (s *Store) GetExercise(id string) (*Exercise, error) {
	var ex Exercise
	var evalMode, referenceCode, criteria sql.NullString
	err := s.db.QueryRow(`
		SELECT id, suite, language, slug, instructions, stub, test_file, test_command, docker_image,
			COALESCE(eval_mode, 'docker-test'), COALESCE(reference_code, ''), COALESCE(criteria, ''), imported_at
		FROM eval_exercises WHERE id = ?`, id,
	).Scan(&ex.ID, &ex.Suite, &ex.Language, &ex.Slug, &ex.Instructions,
		&ex.Stub, &ex.TestFile, &ex.TestCommand, &ex.DockerImage,
		&evalMode, &referenceCode, &criteria, &ex.ImportedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if evalMode.Valid {
		ex.EvalMode = evalMode.String
	}
	if referenceCode.Valid {
		ex.ReferenceCode = referenceCode.String
	}
	if criteria.Valid {
		ex.Criteria = criteria.String
	}
	return &ex, nil
}

// ListExercises returns exercises filtered by optional suite and language.
func (s *Store) ListExercises(suite, language string) ([]Exercise, error) {
	query := `SELECT id, suite, language, slug, instructions, stub, test_file, test_command, docker_image,
		COALESCE(eval_mode, 'docker-test'), COALESCE(reference_code, ''), COALESCE(criteria, ''), imported_at
		FROM eval_exercises WHERE 1=1`
	var args []interface{}

	if suite != "" {
		query += " AND suite = ?"
		args = append(args, suite)
	}
	if language != "" {
		query += " AND language = ?"
		args = append(args, language)
	}
	query += " ORDER BY language, slug"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exercises []Exercise
	for rows.Next() {
		var ex Exercise
		var evalMode, referenceCode, criteria sql.NullString
		if err := rows.Scan(&ex.ID, &ex.Suite, &ex.Language, &ex.Slug, &ex.Instructions,
			&ex.Stub, &ex.TestFile, &ex.TestCommand, &ex.DockerImage,
			&evalMode, &referenceCode, &criteria, &ex.ImportedAt); err != nil {
			return nil, err
		}
		if evalMode.Valid {
			ex.EvalMode = evalMode.String
		}
		if referenceCode.Valid {
			ex.ReferenceCode = referenceCode.String
		}
		if criteria.Valid {
			ex.Criteria = criteria.String
		}
		exercises = append(exercises, ex)
	}
	return exercises, rows.Err()
}

// CountExercises returns the number of exercises matching the filters.
func (s *Store) CountExercises(suite, language string) (int, error) {
	query := "SELECT COUNT(*) FROM eval_exercises WHERE 1=1"
	var args []interface{}
	if suite != "" {
		query += " AND suite = ?"
		args = append(args, suite)
	}
	if language != "" {
		query += " AND language = ?"
		args = append(args, language)
	}
	var count int
	err := s.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// CreateRun creates a new eval run. Thread-safe.
func (s *Store) CreateRun(run EvalRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	configJSON, err := json.Marshal(run.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO eval_runs (id, config, status, started_at)
		VALUES (?, ?, ?, ?)`,
		run.ID, string(configJSON), run.Status, run.StartedAt,
	)
	return err
}

// GetRun retrieves a run by ID.
func (s *Store) GetRun(id string) (*EvalRun, error) {
	var run EvalRun
	var configJSON string
	var completedAt sql.NullTime
	var summaryJSON sql.NullString

	err := s.db.QueryRow(`
		SELECT id, config, status, started_at, completed_at, summary
		FROM eval_runs WHERE id = ?`, id,
	).Scan(&run.ID, &configJSON, &run.Status, &run.StartedAt, &completedAt, &summaryJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(configJSON), &run.Config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	if summaryJSON.Valid {
		var summary EvalSummary
		if err := json.Unmarshal([]byte(summaryJSON.String), &summary); err == nil {
			run.Summary = &summary
		}
	}
	return &run, nil
}

// ListRuns returns recent runs ordered by start time descending.
func (s *Store) ListRuns(limit int) ([]EvalRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, config, status, started_at, completed_at, summary
		FROM eval_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []EvalRun
	for rows.Next() {
		var run EvalRun
		var configJSON string
		var completedAt sql.NullTime
		var summaryJSON sql.NullString

		if err := rows.Scan(&run.ID, &configJSON, &run.Status, &run.StartedAt, &completedAt, &summaryJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(configJSON), &run.Config); err != nil {
			continue
		}
		if completedAt.Valid {
			run.CompletedAt = &completedAt.Time
		}
		if summaryJSON.Valid {
			var summary EvalSummary
			if err := json.Unmarshal([]byte(summaryJSON.String), &summary); err == nil {
				run.Summary = &summary
			}
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// CompleteRun marks a run as completed with a summary.
func (s *Store) CompleteRun(id string, summary *EvalSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var summaryJSON []byte
	if summary != nil {
		var err error
		summaryJSON, err = json.Marshal(summary)
		if err != nil {
			return fmt.Errorf("marshal summary: %w", err)
		}
	}
	_, err := s.db.Exec(`
		UPDATE eval_runs SET status = 'completed', completed_at = ?, summary = ? WHERE id = ?`,
		now, string(summaryJSON), id,
	)
	return err
}

// FailRun marks a run as failed.
func (s *Store) FailRun(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE eval_runs SET status = 'failed', completed_at = ? WHERE id = ?`, now, id)
	return err
}

// InsertResult stores an eval result. Thread-safe for concurrent eval runs.
func (s *Store) InsertResult(result EvalResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fallbackJSON, _ := json.Marshal(result.FallbackChain)
	pass1, pass2 := 0, 0
	if result.Pass1 {
		pass1 = 1
	}
	if result.Pass2 {
		pass2 = 1
	}
	fallbackUsed := 0
	if result.FallbackUsed {
		fallbackUsed = 1
	}
	var metricScore *float64
	var metricName, judgeProvider *string
	if result.MetricScore != 0 {
		metricScore = &result.MetricScore
	}
	if result.MetricName != "" {
		metricName = &result.MetricName
	}
	if result.JudgeProvider != "" {
		judgeProvider = &result.JudgeProvider
	}
	_, err := s.db.Exec(`
		INSERT INTO eval_results (
			id, run_id, exercise_id, provider, model,
			pass_1, pass_2, generated_code, test_output, error_feedback,
			generated_code_2, test_output_2, latency_ms, latency_ms_2,
			prompt_tokens, completion_tokens, total_tokens,
			fallback_used, fallback_chain, docker_exit_code,
			metric_score, metric_name, judge_provider,
			error, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.ID, result.RunID, result.ExerciseID, result.Provider, result.Model,
		pass1, pass2, result.GeneratedCode, result.TestOutput, result.ErrorFeedback,
		result.GeneratedCode2, result.TestOutput2, result.LatencyMs, result.LatencyMs2,
		result.PromptTokens, result.CompletionTokens, result.TotalTokens,
		fallbackUsed, string(fallbackJSON), result.DockerExitCode,
		metricScore, metricName, judgeProvider,
		result.Error, time.Now(),
	)
	return err
}

// GetResultsByRun returns all results for a run.
func (s *Store) GetResultsByRun(runID string) ([]EvalResult, error) {
	rows, err := s.db.Query(`
		SELECT id, run_id, exercise_id, provider, model,
			pass_1, pass_2, generated_code, test_output, error_feedback,
			generated_code_2, test_output_2, latency_ms, latency_ms_2,
			prompt_tokens, completion_tokens, total_tokens,
			fallback_used, fallback_chain, docker_exit_code,
			COALESCE(metric_score, 0), COALESCE(metric_name, ''), COALESCE(judge_provider, ''),
			error, created_at
		FROM eval_results WHERE run_id = ? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EvalResult
	for rows.Next() {
		var r EvalResult
		var pass1, pass2, fallbackUsed int
		var fallbackJSON string
		var model, generatedCode, testOutput, errorFeedback sql.NullString
		var generatedCode2, testOutput2, errStr sql.NullString
		var metricName, judgeProvider sql.NullString

		if err := rows.Scan(
			&r.ID, &r.RunID, &r.ExerciseID, &r.Provider, &model,
			&pass1, &pass2, &generatedCode, &testOutput, &errorFeedback,
			&generatedCode2, &testOutput2, &r.LatencyMs, &r.LatencyMs2,
			&r.PromptTokens, &r.CompletionTokens, &r.TotalTokens,
			&fallbackUsed, &fallbackJSON, &r.DockerExitCode,
			&r.MetricScore, &metricName, &judgeProvider,
			&errStr, &r.CreatedAt,
		); err != nil {
			return nil, err
		}

		r.Pass1 = pass1 == 1
		r.Pass2 = pass2 == 1
		r.FallbackUsed = fallbackUsed == 1
		if model.Valid {
			r.Model = model.String
		}
		if generatedCode.Valid {
			r.GeneratedCode = generatedCode.String
		}
		if testOutput.Valid {
			r.TestOutput = testOutput.String
		}
		if errorFeedback.Valid {
			r.ErrorFeedback = errorFeedback.String
		}
		if generatedCode2.Valid {
			r.GeneratedCode2 = generatedCode2.String
		}
		if testOutput2.Valid {
			r.TestOutput2 = testOutput2.String
		}
		if errStr.Valid {
			r.Error = errStr.String
		}
		if metricName.Valid {
			r.MetricName = metricName.String
		}
		if judgeProvider.Valid {
			r.JudgeProvider = judgeProvider.String
		}
		if fallbackJSON != "" {
			_ = json.Unmarshal([]byte(fallbackJSON), &r.FallbackChain) // best-effort: empty chain on invalid JSON
		}

		results = append(results, r)
	}
	return results, rows.Err()
}

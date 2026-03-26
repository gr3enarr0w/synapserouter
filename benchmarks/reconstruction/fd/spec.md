# fd — Reconstruction Spec

## Overview

A fast, user-friendly alternative to the Unix `find` command. fd uses parallel directory traversal, regex pattern matching by default, and smart defaults (ignores hidden files and .gitignore patterns). Written in Rust for performance and safety.

## Scope

**IN SCOPE:**
- CLI argument parsing (clap derive)
- Parallel recursive directory traversal (using `ignore` crate's WalkBuilder)
- Regex and glob pattern matching on file paths
- Filtering: file type (-t), extension (-e), size (-S), hidden files (-H), max depth (-d)
- Output: colorized (lscolors), plain text, null-separated (-0)
- Smart case sensitivity (case-insensitive unless pattern has uppercase)
- Result buffering with sort, then streaming mode

**OUT OF SCOPE:**
- Command execution (-x/--exec, -X/--exec-batch)
- Time/date filtering (--changed-within/--changed-before)
- Owner filtering (-o/--owner)
- Custom format strings (--format)
- Hyperlink output
- Shell completions
- Batch/job thread pool

**TARGET:** ~1000-1500 LOC Rust, ~12 source files

## Architecture

- **Language/Runtime:** Rust (edition 2021)
- **Key dependencies:**
  - `ignore` (WalkBuilder/WalkParallel for traversal with .gitignore support)
  - `regex` (pattern matching, bytes-based)
  - `clap` (CLI argument parsing, derive macros)
  - `lscolors` (LS_COLORS-based colorized output)
  - `crossbeam-channel` (thread-safe channels for worker/receiver)
  - `globset` (glob-to-regex conversion)

### Directory Structure
```
fd/
  Cargo.toml
  src/
    main.rs           # Entry point, config construction, regex building
    cli.rs            # Clap-derived Opts struct
    config.rs         # Config struct (runtime options)
    walk.rs           # Parallel traversal, worker/receiver loop, filtering
    output.rs         # Result formatting and colorization
    dir_entry.rs      # DirEntry wrapper with lazy metadata
    filesystem.rs     # Path utilities
    filetypes.rs      # FileTypes filter
    filter/
      mod.rs
      size.rs         # SizeFilter (min/max/equals with units)
    regex_helper.rs   # Smart case detection
    error.rs          # Error display utilities
    exit_codes.rs     # ExitCode enum
```

### Design Patterns
- **Producer-consumer:** Parallel walkers (producers) send entries via crossbeam channel to single receiver (consumer)
- **Lazy evaluation:** DirEntry metadata loaded on-demand via OnceCell
- **Buffered→streaming:** Results buffered and sorted initially, then switch to streaming after timeout
- **Filter pipeline:** Sequential filter checks with early return

## Data Flow

```
CLI Args (clap)
  |
  v
main.rs: parse Opts -> build Config -> compile Regex
  |
  v
walk.rs: scan(paths, patterns, config)
  |
  v
WalkBuilder (ignore crate)
  ├── Parallel threads walk filesystem
  ├── .gitignore / .fdignore respected automatically
  └── Hidden files skipped by default
  |
  v
Filter Pipeline (per entry, in parallel workers):
  1. Min/max depth check
  2. Regex pattern match (all patterns must match)
  3. Extension filter (-e)
  4. File type filter (-t file/dir/symlink/empty/executable)
  5. Size filter (-S +100k)
  |
  v
crossbeam-channel -> Receiver
  |
  v
ReceiverBuffer:
  - Buffering mode: collect results, sort by path
  - After timeout (100ms): switch to streaming
  |
  v
output.rs: print entry (colorized or plain)
```

## Core Components

### Opts (src/cli.rs)
- **Purpose:** CLI argument definitions via clap derive
- **Key fields:**
  ```rust
  #[derive(Parser)]
  struct Opts {
      pattern: String,                    // regex pattern (positional, required)
      paths: Vec<PathBuf>,               // search paths (positional, optional)

      #[arg(short = 'H', long)]
      hidden: bool,                       // include hidden files

      #[arg(short = 'I', long)]
      no_ignore: bool,                    // don't respect .gitignore

      #[arg(short, long)]
      case_sensitive: bool,

      #[arg(short = 'i', long)]
      ignore_case: bool,

      #[arg(short = 'g', long)]
      glob: bool,                         // treat pattern as glob

      #[arg(short = 'F', long)]
      fixed_strings: bool,                // treat pattern as literal

      #[arg(short = 'p', long)]
      full_path: bool,                    // match against full path

      #[arg(short, long)]
      r#type: Vec<String>,               // file type filters (f, d, l, x, e)

      #[arg(short, long)]
      extension: Vec<String>,             // extension filters

      #[arg(short = 'S', long)]
      size: Vec<String>,                  // size constraints

      #[arg(short, long)]
      max_depth: Option<usize>,

      #[arg(long)]
      min_depth: Option<usize>,

      #[arg(short = 'E', long)]
      exclude: Vec<String>,               // exclude patterns

      #[arg(short = 'L', long)]
      follow: bool,                       // follow symlinks

      #[arg(short = '0', long)]
      print0: bool,                       // null separator

      #[arg(short = 'a', long)]
      absolute_path: bool,

      #[arg(short = '1')]
      max_one_result: bool,               // stop after first result

      #[arg(long)]
      max_results: Option<usize>,

      #[arg(short = 'q', long)]
      quiet: bool,

      #[arg(short = 'j', long)]
      threads: Option<usize>,

      #[arg(long)]
      prune: bool,                        // don't descend into matching dirs

      #[arg(short = 'c', long, default_value = "auto")]
      color: ColorWhen,                   // auto, always, never
  }
  ```

### Config (src/config.rs)
- **Purpose:** Runtime configuration derived from parsed CLI args
- **Key fields:**
  ```rust
  struct Config {
      case_sensitive: bool,
      ignore_hidden: bool,
      read_fdignore: bool,
      read_vcsignore: bool,
      follow_links: bool,
      null_separator: bool,
      max_depth: Option<usize>,
      min_depth: Option<usize>,
      prune: bool,
      threads: usize,
      max_results: Option<usize>,
      quiet: bool,
      ls_colors: Option<LsColors>,
      file_types: Option<FileTypes>,
      extensions: Option<RegexSet>,
      size_constraints: Vec<SizeFilter>,
      exclude_patterns: Vec<String>,
      strip_cwd_prefix: bool,
      cwd: PathBuf,
  }
  ```

### DirEntry (src/dir_entry.rs)
- **Purpose:** Wrapper around ignore::DirEntry with lazy metadata
- **Key implementation:**
  ```rust
  struct DirEntry {
      inner: DirEntryInner,           // Normal(ignore::DirEntry) | BrokenSymlink(PathBuf)
      metadata: OnceCell<Option<Metadata>>,
      style: OnceCell<Option<Style>>,  // cached lscolors style
  }

  impl DirEntry {
      fn path(&self) -> &Path
      fn file_type(&self) -> Option<FileType>
      fn metadata(&self) -> Option<&Metadata>
      fn depth(&self) -> usize
      fn style(&self, ls_colors: &LsColors) -> Option<&Style>
  }

  impl Ord for DirEntry  // for sorting buffered results
  ```

### FileTypes (src/filetypes.rs)
- **Purpose:** Filter entries by file type
- **Implementation:**
  ```rust
  struct FileTypes {
      files: bool,
      directories: bool,
      symlinks: bool,
      executables_only: bool,
      empty_only: bool,
  }

  impl FileTypes {
      fn should_ignore(&self, entry: &DirEntry) -> bool
  }
  ```
- **Type flags:** `-t f` (file), `-t d` (dir), `-t l` (symlink), `-t x` (executable), `-t e` (empty)

### SizeFilter (src/filter/size.rs)
- **Purpose:** Filter files by size
- **Implementation:**
  ```rust
  enum SizeFilter {
      Min(u64),
      Max(u64),
      Equals(u64),
  }

  impl SizeFilter {
      fn parse(s: &str) -> Result<SizeFilter>
      fn is_within(&self, size: u64) -> bool
  }
  ```
- **Parsing format:** `[+|-]NUM[b|k|ki|m|mi|g|gi|t|ti]`
  - `+` = minimum, `-` = maximum, no prefix = exact
  - SI units: k=1000, m=1e6, g=1e9, t=1e12
  - Binary: ki=1024, mi=1048576, gi=1073741824, ti=1099511627776

### Walk/Scan (src/walk.rs)
- **Purpose:** Core traversal, filtering, and result delivery
- **Key functions:**
  ```rust
  fn scan(paths: &[PathBuf], patterns: &[Regex], config: &Config) -> ExitCode
  ```
- **Architecture:**
  1. Build WalkBuilder from `ignore` crate with config options
  2. Spawn parallel walkers, each runs filter pipeline
  3. Send matching entries via crossbeam channel
  4. Receiver collects in buffer, sorts, then prints
  5. After `max_buffer_time` (100ms), switch to streaming mode

### Output (src/output.rs)
- **Purpose:** Format and print matching entries
- **Key functions:**
  ```rust
  fn print_entry(entry: &DirEntry, config: &Config)
  fn print_entry_colorized(entry: &DirEntry, config: &Config, ls_colors: &LsColors)
  fn print_entry_uncolorized(entry: &DirEntry, config: &Config)
  ```
- **Colorized output:** Split path into parent + filename, style each with lscolors
- **Null separator:** Use `\0` instead of `\n` when `-0` flag set

### RegexHelper (src/regex_helper.rs)
- **Purpose:** Smart case detection
- **Logic:** If pattern contains any uppercase character, enable case-sensitive matching (unless overridden)

## Configuration

CLI flags only (no config file). Key defaults:
- Pattern: regex (not glob)
- Case: smart case (insensitive unless pattern has uppercase)
- Hidden files: ignored
- .gitignore: respected
- Max depth: unlimited
- Threads: number of CPUs
- Color: auto (if terminal)
- Separator: newline

## Test Cases

### Functional Tests
1. **Simple pattern:** `fd test` in test dir -> finds files matching "test"
2. **Case insensitive:** `fd readme` -> finds README.md (smart case)
3. **Case sensitive override:** `fd -s readme` -> finds nothing (no lowercase readme)
4. **Glob mode:** `fd -g '*.rs'` -> finds all Rust files
5. **Fixed string:** `fd -F 'foo.bar'` -> matches literal dot (not regex any-char)
6. **Extension filter:** `fd -e rs` -> finds only .rs files
7. **Type filter (files):** `fd -t f pattern` -> only files, no directories
8. **Type filter (dirs):** `fd -t d src` -> only directories named src
9. **Hidden files:** `fd -H .gitignore` -> finds .gitignore (normally hidden)
10. **Max depth:** `fd -d 1 .` -> only top-level entries
11. **Size filter:** `fd -S +1k -t f` -> files larger than 1KB
12. **Full path match:** `fd -p src/main` -> matches against full path
13. **Null separator:** `fd -0 pattern` -> entries separated by \0

### Edge Cases
1. **No matches:** `fd nonexistent_pattern` -> exit code 1, no output
2. **Empty directory:** traversal handles dirs with no entries
3. **Broken symlinks:** reported or skipped gracefully
4. **Multiple patterns:** `fd pattern1 --and pattern2` -> both must match

## Build & Run

### Build
```bash
cargo build --release
```

### Run
```bash
# Find all .rs files
./target/release/fd -e rs

# Find directories named "src"
./target/release/fd -t d src

# Case-sensitive search for "README"
./target/release/fd -s README

# Include hidden files, ignore .gitignore
./target/release/fd -H -I pattern

# Find files larger than 1MB
./target/release/fd -S +1m -t f
```

### Test
```bash
cargo test
```

## Acceptance Criteria

1. Project builds with `cargo build` without errors
2. Basic regex pattern matching works (finds files by name)
3. Smart case: lowercase pattern = insensitive, uppercase = sensitive
4. Glob mode (`-g`) converts glob patterns to regex correctly
5. Fixed string mode (`-F`) escapes regex special characters
6. Extension filter (`-e rs`) matches file extensions
7. Type filter (`-t f/d/l/x/e`) correctly filters by file type
8. Size filter (`-S +1k/-1m`) parses units and filters correctly
9. Hidden files excluded by default, included with `-H`
10. .gitignore patterns respected by default, disabled with `-I`
11. Max depth (`-d N`) limits traversal depth
12. Parallel traversal works (uses multiple threads)
13. Colorized output using LS_COLORS (when terminal detected)
14. Null separator (`-0`) works correctly
15. Exit code: 0 if matches found, 1 if no matches

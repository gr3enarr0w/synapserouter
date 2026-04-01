# Router Reconstruction Test Suite

75 projects across 15 categories used to **find and fix weaknesses in the synapserouter agent**. Each project that fails reveals a gap — wrong tool usage, bad architecture decisions, missing skill knowledge, escalation problems, wrong output format, etc. We fix the router, re-run, and iterate until it handles all 75.

This is not a scorecard. It's a test-driven improvement plan for the router.

## Structure

Each project gets its own folder under `~/Development/` with a `spec.md`:

```
~/Development/
  spring-petclinic/
    spec.md           # Written by Claude Code (examiner)
    synroute.md       # Created by synapserouter (tracks state)
    src/              # Built by synapserouter (test subject)
  umami/
    spec.md
    ...
```

Review repos cloned to `~/Development/Project_Review/` for examiner reference.

- **spec.md** — Written by Claude Code after analyzing the original repo
- **synroute.md** — Agent's project state file, created/updated each run
- Synapserouter NEVER sees the original repos. Only the spec.

## Projects

### Source Code Languages (35 projects)

#### Wave 1: Calibration (one per language with skills)
| # | Project | Language | Spec | Tested |
|---|---------|----------|------|--------|
| 1 | spring-petclinic | Java | Done | - |
| 2 | umami | TypeScript | Done | - |
| 3 | fd | Rust | Done | Testing |
| 4 | goodbooks-10k | ML Python | Done | - |
| 5 | restic | Go | Done | - |

#### Wave 2: Skill Gaps
| # | Project | Language | Spec | Tested |
|---|---------|----------|------|--------|
| 6 | duplicati | C# | Done | - |
| 7 | end-to-end-diamond-price-prediction | ML Python | Done | PASS (R²=0.981) |
| 8 | httpie-cli | Python | Done | - |
| 9 | quarkus-quickstarts | Java | Done | - |
| 10 | bat | Rust | Done | - |

#### Wave 3: Architecture Complexity
| # | Project | Language | Spec |
|---|---------|----------|------|
| 11-20 | caddy, n8n, outline, cli-cli, credit-card-fraud, jhipster, java-design-patterns, eshop, directus, starship | Mixed | Done |

#### Wave 4: Scale
| # | Project | Language | Spec |
|---|---------|----------|------|
| 21-30 | ripgrep, cal-com, prefect, streamlit, syncthing, rclone, vaultwarden, stock-prediction, handson-ml2, sharex | Mixed | Done |

#### Wave 5: Boss Level
| # | Project | Language | Spec |
|---|---------|----------|------|
| 31-35 | yt-dlp, home-assistant-core, airbyte, jellyfin, powershell | Mixed | Done |

### Jupyter Notebooks — .ipynb (5 projects)

Tests the agent's ability to generate structured JSON notebook files with code cells, markdown cells, and outputs.

| # | Project | What It Tests | Difficulty |
|---|---------|--------------|------------|
| 36 | traffic-sign-recognition | CNN image classification, data augmentation, GTSRB dataset | Hard |
| 37 | sentiment-analysis-lstm | NLP, RNN/LSTM, word embeddings, movie reviews | Hard |
| 38 | cifar10-cnn | Computer vision, Keras CNN, hyperparameter tuning | Medium |
| 39 | movie-recommender-knn | KNN from scratch, collaborative filtering | Medium |
| 40 | human-activity-recognition | 1D CNN on time-series sensor data | Hard |

Reference: [chhayac/Machine-Learning-Notebooks](https://github.com/chhayac/Machine-Learning-Notebooks), [fchollet/deep-learning-with-python-notebooks](https://github.com/fchollet/deep-learning-with-python-notebooks)

### R Markdown — .Rmd (5 projects)

Tests the agent's ability to generate R code chunks, ggplot2 visualizations, and reproducible statistical reports.

| # | Project | What It Tests | Difficulty |
|---|---------|--------------|------------|
| 41 | wine-quality-eda | EDA, distributions, outliers, multivariate analysis, ggplot2 | Medium |
| 42 | co2-emissions-analysis | 12 visualizations, linear regression, OWID data | Medium |
| 43 | electricity-load-forecasting | Time-series forecasting, R-Shiny dashboard | Hard |
| 44 | icu-kidney-injury-prediction | Clinical statistics, logistic regression, survival analysis | Hard |
| 45 | reproducible-research-template | Academic paper pipeline, BibTeX, Makefile, LaTeX | Medium |

Reference: [Ashish25/EDA_Rmd](https://github.com/Ashish25/EDA_Rmd), [hyunjoonbok/R-projects](https://github.com/hyunjoonbok/R-projects)

### SQL / Database (5 projects)

Tests the agent's ability to generate DDL schemas, complex queries, stored procedures, triggers, and migrations.

| # | Project | What It Tests | Difficulty |
|---|---------|--------------|------------|
| 46 | data-warehouse-etl | Star schema, ETL pipeline (Bronze→Silver→Gold), CTEs, window functions | Hard |
| 47 | railway-reservation-system | PostgreSQL, PL/pgSQL, row-level security, triggers | Hard |
| 48 | hospital-management | MySQL, ER design, normalization, complex joins, views | Medium |
| 49 | restaurant-management-db | Schema design, 30 query exercises, indexing | Medium |
| 50 | bank-account-system | PostgreSQL, transactions, isolation levels, constraints | Hard |

Reference: [tushar2704/SQL-Portfolio](https://github.com/tushar2704/SQL-Portfolio), [ptyadana/SQL-Data-Analysis-and-Visualization-Projects](https://github.com/ptyadana/SQL-Data-Analysis-and-Visualization-Projects)

### Wave 6: New Languages (25 projects)

Tests the agent's ability to work with languages not in the original 50. Each requires new or enhanced skills.

#### Kotlin
| # | Project | What It Tests | Difficulty | Reference |
|---|---------|--------------|------------|----------|
| 51 | ktor-sample-app | Ktor web framework, coroutines, REST API | Medium | [ktorio/ktor-samples](https://github.com/ktorio/ktor-samples) |
| 52 | kotlin-android-weather | Android MVVM, Retrofit, Room DB | Hard | [lynnemunini/forecast-app](https://github.com/lynnemunini/forecast-app) |
| 53 | kotlin-spring-boot-api | Spring Boot + Kotlin, data classes, coroutines | Medium | [callicoder/kotlin-spring-boot-jpa-rest-api-demo](https://github.com/callicoder/kotlin-spring-boot-jpa-rest-api-demo) |
| 54 | kotlin-multiplatform-calculator | KMP targeting JVM + native | Hard | [InsertKoinIO/koin](https://github.com/InsertKoinIO/koin) |
| 55 | kotlin-dsl-builder | Type-safe DSL builder, Gradle plugin | Medium | [Kotlin/kotlinx.html](https://github.com/Kotlin/kotlinx.html) |

#### Swift
| # | Project | What It Tests | Difficulty | Reference |
|---|---------|--------------|------------|----------|
| 56 | vapor-todo-api | Vapor web framework, Fluent ORM, REST API | Medium | [iq3addLi/swift-vapor-layered-realworld-example-app](https://github.com/iq3addLi/swift-vapor-layered-realworld-example-app) |
| 57 | swift-cli-tool | ArgumentParser, async/await, file I/O | Medium | [swiftlang/swift-getting-started-cli](https://github.com/swiftlang/swift-getting-started-cli) |
| 58 | swiftui-weather-app | SwiftUI, Combine, CoreLocation, API integration | Hard | [Starkrimson/WeatherApp](https://github.com/Starkrimson/WeatherApp) |
| 59 | swift-data-structures | Generic collections, protocols, Codable | Medium | [kodecocodes/swift-algorithm-club](https://github.com/kodecocodes/swift-algorithm-club) |
| 60 | swift-package-library | SPM library, protocol-oriented design, DocC | Medium | [Alamofire/Alamofire](https://github.com/Alamofire/Alamofire) |

#### C++
| # | Project | What It Tests | Difficulty | Reference |
|---|---------|--------------|------------|----------|
| 61 | cpp-http-server | Boost.Beast/httplib, REST endpoints, JSON | Hard | [yhirose/cpp-httplib](https://github.com/yhirose/cpp-httplib) |
| 62 | cpp-raytracer | Ray tracing, linear algebra, PPM output | Medium | [ssloy/tinyraytracer](https://github.com/ssloy/tinyraytracer) |
| 63 | cpp-game-engine-2d | SDL2/SFML, ECS architecture, sprite rendering | Hard | [GarageGames/Torque2D](https://github.com/GarageGames/Torque2D) |
| 64 | cpp-data-structures | Templates, iterators, STL-compatible containers | Medium | [Sunrisepeak/dstruct](https://github.com/Sunrisepeak/dstruct) |
| 65 | cpp-build-system | CMake, vcpkg/Conan, cross-platform, GTest | Medium | [kigster/cmake-project-template](https://github.com/kigster/cmake-project-template) |

#### Ruby
| # | Project | What It Tests | Difficulty | Reference |
|---|---------|--------------|------------|----------|
| 66 | rails-blog-api | Rails 7 API mode, PostgreSQL, JWT auth | Medium | [loopstudio/rails-api-boilerplate](https://github.com/loopstudio/rails-api-boilerplate) |
| 67 | ruby-cli-gem | Thor/OptionParser, gem packaging, RSpec | Medium | [sparklemotion/mechanize](https://github.com/sparklemotion/mechanize) |
| 68 | sinatra-microservice | Sinatra REST API, Redis, background jobs | Medium | [nmattisson/sinatra-warden-api](https://github.com/nmattisson/sinatra-warden-api) |
| 69 | ruby-web-scraper | Nokogiri, Mechanize, concurrent scraping | Medium | [Skarlso/rscrap](https://github.com/Skarlso/rscrap) |
| 70 | rails-graphql-api | GraphQL-Ruby, N+1 prevention, subscriptions | Hard | [loopstudio/rails-graphql-api-boilerplate](https://github.com/loopstudio/rails-graphql-api-boilerplate) |

#### PHP
| # | Project | What It Tests | Difficulty | Reference |
|---|---------|--------------|------------|----------|
| 71 | laravel-rest-api | Laravel 11, Eloquent, API resources, Sanctum auth | Medium | [Lomkit/laravel-rest-api](https://github.com/Lomkit/laravel-rest-api) |
| 72 | symfony-crud-app | Symfony 7, Doctrine ORM, Twig templates | Medium | [daveh/symfony-crud-example](https://github.com/daveh/symfony-crud-example) |
| 73 | php-cli-tool | Symfony Console, PSR standards, Composer package | Medium | [adhocore/php-cli](https://github.com/adhocore/php-cli) |
| 74 | wordpress-custom-plugin | Plugin API, custom post types, REST endpoints | Medium | [WPBP/WordPress-Plugin-Boilerplate-Powered](https://github.com/WPBP/WordPress-Plugin-Boilerplate-Powered) |
| 75 | php-queue-worker | Laravel queues, Redis, retry/backoff, monitoring | Hard | [laravel/horizon](https://github.com/laravel/horizon) |

## Skills Required Per Category

| Category | Existing Skills | New Skills Needed |
|----------|----------------|-------------------|
| Go | go-patterns, go-testing | - |
| Python | python-patterns, python-testing | - |
| ML Python | ml-patterns, python-patterns | - |
| Rust | rust-patterns | - |
| TypeScript | javascript-patterns | - |
| Java | java-spring | - |
| C# | csharp-patterns | - |
| Jupyter (.ipynb) | ml-patterns | notebook-patterns (NEW) |
| R Markdown (.Rmd) | - | r-patterns (NEW) |
| SQL | sql-expert | sql-expert verify commands (NEW) |
| Kotlin | java-spring (partial) | kotlin-patterns (NEW) |
| Swift | - | swift-patterns (NEW) |
| C++ | - | cpp-patterns (NEW) |
| Ruby | - | ruby-patterns (NEW) |
| PHP | - | php-patterns (NEW) |

## Workflow

1. Clone original repo to `~/Development/Project_Review/`
2. Analyze functionality, write `~/Development/<project>/spec.md`
3. Synapserouter builds from spec:
   ```bash
   cd ~/Development/<project>
   synroute chat --spec-file spec.md
   ```
4. When the router fails or produces bad output → that's a router bug
5. Fix the router (skills, pipeline, escalation, tools, prompts)
6. Re-run the same project to verify the fix
7. Move to next project, repeat

## Failure Categories

| Category | Description | Example Fix |
|----------|-------------|-------------|
| `skill_gap` | No skill for language/framework | Create new skill .md |
| `architecture` | Wrong patterns chosen | Add patterns to skill |
| `tool_usage` | Tools used incorrectly | Fix tool implementation |
| `escalation` | Stuck on weak model | Fix escalation thresholds |
| `pipeline` | Skipped phases | Fix pipeline enforcement |
| `scope` | Overwhelmed by project size | Improve task decomposition |
| `context` | Lost context mid-task | Fix context management |
| `dependency` | Can't set up build env | Improve environment detection |
| `output_format` | Wrong file format (ipynb/Rmd/SQL) | Add format-specific skill |
| `destructive` | Agent deleted/destroyed work | Add safety guards to tools |

## Test Results

| # | Project | Language | Result | Bugs Found |
|---|---------|----------|--------|------------|
| 7 | diamond-price-prediction | ML Python | PASS (R²=0.981, 6/6 phases) | 11 router bugs fixed |
| 3 | fd | Rust | IN PROGRESS | Bash timeout fix, rm -rf safety |
| 1 | spring-petclinic | Java | FAIL (compiles: no, self-check loop, scope drift) | 4 router bugs filed |

## Router Bugs Found & Fixed

| Bug | Category | Fix |
|-----|----------|-----|
| Models call wrong tool names | tool_usage | Blocklist in system prompt |
| Agent exits after plan phase | pipeline | Removed toolCallCount gate |
| Memory injects 163 msgs (overflow) | context | 8K cap, negative clamp |
| Level 0 models can't function-call | escalation | Stall detection → escalate |
| Pipeline infinite loop on quality gate | pipeline | Phase retry cap (5→escalate, 10→skip) |
| Sub-agent uses wrong provider | escalation | Children inherit escalation chain |
| Phase signals ignored during tool calls | pipeline | Check advancePipeline after tools |
| Agent runs full ML training | tool_usage | Execution rules in prompt |
| PhasePrompt returns raw %s | pipeline | Descriptive placeholder |
| Pipeline phases never engage | pipeline | Inject phase prompt at startup |
| Bash timeout doesn't kill children | tool_usage | Start()+Wait()+select pattern |

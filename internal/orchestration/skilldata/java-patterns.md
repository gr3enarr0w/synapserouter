---
name: java-patterns
description: "Modern Java development (17+) — records, sealed classes, streams, virtual threads."
triggers:
  - "java"
  - ".java"
  - "jdk"
  - "maven"
  - "javac"
  - "openjdk"
  - "gradle+java"
role: coder
phase: analyze
language: java
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "compile"
    command: "mvn compile 2>&1 || gradle compileJava 2>&1 || javac *.java 2>&1 || echo 'COMPILE_FAILED'"
    expect_not: "COMPILE_FAILED"
---
# java-patterns

## Modern Java (17+)
- **Records** — immutable data carriers (replace POJOs)
- **Sealed classes** — restricted inheritance hierarchies
- **Pattern matching** — instanceof with binding, switch expressions
- **Text blocks** — multi-line strings with """
- **var** — local variable type inference
- **Virtual threads** (21+) — lightweight threads for I/O-bound work

## Core Patterns
- Optional over null returns
- Stream API for collection processing (map, filter, reduce)
- CompletableFuture for async composition
- Dependency injection via constructor (not field injection)
- Builder pattern for complex object construction
- Immutability by default (final fields, unmodifiable collections)

## Build Tools
- Maven: pom.xml, lifecycle phases (compile, test, package)
- Gradle: build.gradle.kts (Kotlin DSL preferred), tasks
- Both: dependency management, BOM imports

## Anti-Patterns
- Raw types (always parameterize generics)
- Checked exception abuse (use RuntimeException subtypes)
- Null returns (use Optional)
- Mutable singletons
- Field injection (@Autowired on fields)
- God classes (single responsibility principle)

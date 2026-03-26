---
name: code-implement
description: "Produce implementation-ready code changes."
triggers:
  - "implement"
  - "build"
  - "write"
  - "refactor"
  - "set up"
  - "setup"
role: coder
phase: implement
verify:
  - name: "compiles without errors"
    command: "go build ./... 2>&1 || echo 'BUILD_FAILED'"
    expect_not: "BUILD_FAILED"
  - name: "go vet passes"
    command: "go vet ./... 2>&1 || echo 'VET_FAILED'"
    expect_not: "VET_FAILED"
  - name: "no compile warnings"
    command: "go build ./... 2>&1 | grep -i 'warning\\|unused\\|declared and not used' || echo 'OK'"
    expect: "OK"
---
# code-implement



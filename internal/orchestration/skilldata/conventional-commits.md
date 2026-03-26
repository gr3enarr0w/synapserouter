---
name: conventional-commits
description: "Conventional commits, changelog generation, semantic versioning, release automation."
triggers:
  - "conventional commit"
  - "semantic version"
  - "changelog"
  - "release"
  - "semver"
  - "commit message"
  - "goreleaser"
  - "semantic-release"
role: coder
phase: implement
verify:
  - name: "commit format"
    command: "git log -1 --format='%s' | grep -E '^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\\(.+\\))?: .+' || echo 'BAD_FORMAT'"
    expect_not: "BAD_FORMAT"
---
# conventional-commits

## Format
```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

## Types
| Type | When |
|------|------|
| feat | New feature (bumps minor) |
| fix | Bug fix (bumps patch) |
| docs | Documentation only |
| refactor | Code change that neither fixes nor adds |
| test | Adding/fixing tests |
| ci | CI/CD changes |
| chore | Maintenance |
| perf | Performance improvement |
| BREAKING CHANGE | In footer — bumps major |

## Release Automation
- **semantic-release**: auto-version from commits, publish to npm/GitHub
- **goreleaser**: Go binary releases with changelog
- **standard-version**: changelog generation without publishing
- **release-please**: Google's GitHub Action for release PRs

## Anti-Patterns
- Vague messages ("fix stuff", "update code")
- Mixing concerns in one commit
- Missing type prefix
- Not using scopes for monorepos

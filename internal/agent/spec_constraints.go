package agent

import (
	"regexp"
	"strings"
)

// SpecConstraints holds key architectural constraints extracted from a spec.
type SpecConstraints struct {
	PackageStructure string   // e.g., "org.springframework.samples.petclinic"
	OutOfScope       []string // items explicitly excluded
	Prohibited       []string // patterns explicitly forbidden
	InScope          []string // items explicitly included
	DirectoryLayout  string   // required directory structure
}

// ExtractSpecConstraints parses a spec document for key architectural constraints.
// Uses regex extraction -- no LLM call needed, works at session startup.
func ExtractSpecConstraints(spec string) *SpecConstraints {
	c := &SpecConstraints{}

	// Extract package structure — supports all languages:
	// - Java/Kotlin: "org.springframework.samples.petclinic" (dot-separated, 3+ segments)
	// - Go/Python/Rust: "calc", "my_app", "petclinic" (single word after "Package:")
	// - Directory paths: "java/org/springframework/samples/petclinic/"

	// Try dot-separated first (Java-style: org.xxx.yyy)
	pkgRe := regexp.MustCompile(`(?i)(?:package[:\s]+)([a-z][a-z0-9_.]+(?:\.[a-z][a-z0-9_]+){2,})`)
	if m := pkgRe.FindStringSubmatch(spec); len(m) > 1 {
		c.PackageStructure = m[1]
	}

	// Try single-word package (Go/Python/Rust: "Package: calc")
	if c.PackageStructure == "" {
		singlePkgRe := regexp.MustCompile(`(?im)^(?:\*\*)?Package(?:\*\*)?[:\s]+([a-zA-Z][a-zA-Z0-9_-]+)`)
		if m := singlePkgRe.FindStringSubmatch(spec); len(m) > 1 {
			// Don't capture generic words
			pkg := strings.TrimSpace(m[1])
			if pkg != "structure" && pkg != "name" && pkg != "manager" && pkg != "the" {
				c.PackageStructure = pkg
			}
		}
	}

	// Try path-based extraction: java/org/springframework/samples/petclinic/
	if c.PackageStructure == "" {
		pathRe := regexp.MustCompile(`java/([a-z][a-z0-9_]+(?:/[a-z][a-z0-9_]+){2,})`)
		if m := pathRe.FindStringSubmatch(spec); len(m) > 1 {
			c.PackageStructure = strings.ReplaceAll(m[1], "/", ".")
		}
	}

	// Extract OUT OF SCOPE — supports both bullet lists AND inline comma-separated
	outRe := regexp.MustCompile(`(?i)(?:OUT\s+OF\s+SCOPE|OUT.SCOPE)[:\s]*\n((?:[-*]\s+.+\n?)+)`)
	if m := outRe.FindStringSubmatch(spec); len(m) > 1 {
		for _, line := range strings.Split(m[1], "\n") {
			line = strings.TrimSpace(line)
			line = strings.TrimLeft(line, "-* ")
			if line != "" {
				c.OutOfScope = append(c.OutOfScope, line)
			}
		}
	}
	// Fallback: inline format "OUT OF SCOPE: item1, item2, item3"
	if len(c.OutOfScope) == 0 {
		inlineOutRe := regexp.MustCompile(`(?i)OUT\s+OF\s+SCOPE[:\s]+([^\n]+)`)
		if m := inlineOutRe.FindStringSubmatch(spec); len(m) > 1 {
			for _, item := range strings.Split(m[1], ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					c.OutOfScope = append(c.OutOfScope, item)
				}
			}
		}
	}

	// Extract IN SCOPE — supports both bullet lists AND inline
	inRe := regexp.MustCompile(`(?i)(?:IN\s+SCOPE|IN.SCOPE)[:\s]*\n((?:[-*]\s+.+\n?)+)`)
	if m := inRe.FindStringSubmatch(spec); len(m) > 1 {
		for _, line := range strings.Split(m[1], "\n") {
			line = strings.TrimSpace(line)
			line = strings.TrimLeft(line, "-* ")
			if line != "" {
				c.InScope = append(c.InScope, line)
			}
		}
	}
	// Fallback: inline format "IN SCOPE: item1, item2"
	if len(c.InScope) == 0 {
		inlineInRe := regexp.MustCompile(`(?i)IN\s+SCOPE[:\s]+([^\n]+)`)
		if m := inlineInRe.FindStringSubmatch(spec); len(m) > 1 {
			for _, item := range strings.Split(m[1], ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					c.InScope = append(c.InScope, item)
				}
			}
		}
	}

	// Extract prohibited patterns (look for "no service layer", "do not", "must not", etc.)
	prohibRe := regexp.MustCompile(`(?i)(?:no\s+service\s+layer|do\s+not\s+[^.]+|must\s+not\s+[^.]+|without\s+[^.]+layer)`)
	for _, m := range prohibRe.FindAllString(spec, -1) {
		c.Prohibited = append(c.Prohibited, strings.TrimSpace(m))
	}

	// Extract directory layout (look for tree-like structures)
	dirRe := regexp.MustCompile(`(?m)^[\s]*[a-z]+/\n(?:[\s]+[├└│─\s]*[a-z].+\n?)+`)
	if m := dirRe.FindString(spec); m != "" {
		c.DirectoryLayout = strings.TrimSpace(m)
	}

	return c
}

// FormatConstraints returns the constraints as a prominently formatted block
// suitable for injection into system prompts.
func (c *SpecConstraints) FormatConstraints() string {
	if c == nil || (c.PackageStructure == "" && len(c.OutOfScope) == 0 && len(c.Prohibited) == 0 && len(c.InScope) == 0) {
		return ""
	}

	var b strings.Builder
	b.WriteString("SPEC CONSTRAINTS (MANDATORY -- override any conflicting skill pattern):\n")

	if c.PackageStructure != "" {
		b.WriteString("  PACKAGE: " + c.PackageStructure + "\n")
	}
	if len(c.InScope) > 0 {
		b.WriteString("  IN SCOPE:\n")
		for _, item := range c.InScope {
			b.WriteString("    + " + item + "\n")
		}
	}
	if len(c.Prohibited) > 0 {
		b.WriteString("  PROHIBITED:\n")
		for _, p := range c.Prohibited {
			b.WriteString("    X " + p + "\n")
		}
	}
	if len(c.OutOfScope) > 0 {
		b.WriteString("  OUT OF SCOPE (do NOT implement):\n")
		for _, item := range c.OutOfScope {
			b.WriteString("    X " + item + "\n")
		}
	}
	if c.DirectoryLayout != "" {
		b.WriteString("  REQUIRED DIRECTORY LAYOUT:\n" + c.DirectoryLayout + "\n")
	}

	return b.String()
}

// IsEmpty returns true if no constraints were extracted.
func (c *SpecConstraints) IsEmpty() bool {
	if c == nil {
		return true
	}
	return c.PackageStructure == "" && len(c.OutOfScope) == 0 &&
		len(c.Prohibited) == 0 && len(c.InScope) == 0 && c.DirectoryLayout == ""
}

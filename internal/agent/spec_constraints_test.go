package agent

import (
	"strings"
	"testing"
)

func TestExtractSpecConstraints_PackageStructure(t *testing.T) {
	spec := `Build a Spring PetClinic application.
Package: org.springframework.samples.petclinic
Use Maven for builds.`

	c := ExtractSpecConstraints(spec)
	if c.PackageStructure != "org.springframework.samples.petclinic" {
		t.Errorf("PackageStructure = %q, want %q", c.PackageStructure, "org.springframework.samples.petclinic")
	}
}

func TestExtractSpecConstraints_PackageFromPath(t *testing.T) {
	spec := `Create files under java/org/springframework/samples/petclinic/ directory.`
	c := ExtractSpecConstraints(spec)
	if c.PackageStructure != "org.springframework.samples.petclinic" {
		t.Errorf("PackageStructure = %q, want %q", c.PackageStructure, "org.springframework.samples.petclinic")
	}
}

func TestExtractSpecConstraints_OutOfScope(t *testing.T) {
	spec := `## OUT OF SCOPE:
- Service layer abstraction
- Security/authentication
- Database migrations
`
	c := ExtractSpecConstraints(spec)
	if len(c.OutOfScope) != 3 {
		t.Fatalf("OutOfScope has %d items, want 3: %v", len(c.OutOfScope), c.OutOfScope)
	}
	if c.OutOfScope[0] != "Service layer abstraction" {
		t.Errorf("OutOfScope[0] = %q, want %q", c.OutOfScope[0], "Service layer abstraction")
	}
}

func TestExtractSpecConstraints_InScope(t *testing.T) {
	spec := `## IN SCOPE:
- REST controllers
- JPA entities
- Thymeleaf templates
`
	c := ExtractSpecConstraints(spec)
	if len(c.InScope) != 3 {
		t.Fatalf("InScope has %d items, want 3: %v", len(c.InScope), c.InScope)
	}
	if c.InScope[0] != "REST controllers" {
		t.Errorf("InScope[0] = %q, want %q", c.InScope[0], "REST controllers")
	}
}

func TestExtractSpecConstraints_Prohibited(t *testing.T) {
	spec := `Build controllers directly, no service layer. Do not add authentication. Must not create DTOs.`
	c := ExtractSpecConstraints(spec)
	if len(c.Prohibited) == 0 {
		t.Fatal("expected prohibited patterns, got none")
	}
	found := false
	for _, p := range c.Prohibited {
		if strings.Contains(strings.ToLower(p), "no service layer") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'no service layer' in prohibited, got: %v", c.Prohibited)
	}
}

func TestExtractSpecConstraints_EmptySpec(t *testing.T) {
	c := ExtractSpecConstraints("short message")
	if !c.IsEmpty() {
		t.Errorf("expected empty constraints for short input, got: package=%q, in=%d, out=%d, prohib=%d",
			c.PackageStructure, len(c.InScope), len(c.OutOfScope), len(c.Prohibited))
	}
}

func TestFormatConstraints_Empty(t *testing.T) {
	c := &SpecConstraints{}
	if c.FormatConstraints() != "" {
		t.Error("expected empty string for empty constraints")
	}
}

func TestFormatConstraints_Full(t *testing.T) {
	c := &SpecConstraints{
		PackageStructure: "org.example.app",
		OutOfScope:       []string{"Security", "Migrations"},
		Prohibited:       []string{"no service layer"},
		InScope:          []string{"REST API", "JPA entities"},
	}
	formatted := c.FormatConstraints()
	if !strings.Contains(formatted, "SPEC CONSTRAINTS") {
		t.Error("missing SPEC CONSTRAINTS header")
	}
	if !strings.Contains(formatted, "org.example.app") {
		t.Error("missing package structure")
	}
	if !strings.Contains(formatted, "Security") {
		t.Error("missing out of scope item")
	}
	if !strings.Contains(formatted, "no service layer") {
		t.Error("missing prohibited item")
	}
	if !strings.Contains(formatted, "REST API") {
		t.Error("missing in scope item")
	}
}

func TestExtractSpecConstraints_PetClinicLike(t *testing.T) {
	spec := `# Spring PetClinic Reconstruction

Build a Spring Boot PetClinic application with the following structure.
Package: org.springframework.samples.petclinic

## IN SCOPE:
- Owner/Pet/Vet/Visit JPA entities
- REST controllers for CRUD operations
- Thymeleaf HTML templates
- H2 in-memory database

## OUT OF SCOPE:
- Service layer abstraction (controllers talk directly to repositories)
- Spring Security / authentication
- Database migrations (use JPA auto-DDL)
- Docker containerization

Architecture: no service layer. Controllers inject repositories directly.
Do not add DTO classes. Must not create separate configuration modules.

Directory structure:
src/
  main/
    java/org/springframework/samples/petclinic/
      owner/
      pet/
      vet/
      visit/
`
	c := ExtractSpecConstraints(spec)

	if c.PackageStructure != "org.springframework.samples.petclinic" {
		t.Errorf("PackageStructure = %q", c.PackageStructure)
	}
	if len(c.InScope) != 4 {
		t.Errorf("InScope = %d items, want 4: %v", len(c.InScope), c.InScope)
	}
	if len(c.OutOfScope) != 4 {
		t.Errorf("OutOfScope = %d items, want 4: %v", len(c.OutOfScope), c.OutOfScope)
	}
	if len(c.Prohibited) == 0 {
		t.Error("expected prohibited patterns")
	}
	if c.IsEmpty() {
		t.Error("constraints should not be empty")
	}

	formatted := c.FormatConstraints()
	if formatted == "" {
		t.Error("formatted constraints should not be empty")
	}
	t.Logf("Formatted output:\n%s", formatted)
}

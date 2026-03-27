---
name: java-spring
description: "Spring Boot 3.x development — JPA, constructor injection, layered architecture."
triggers:
  - "java"
  - "spring"
  - "spring boot"
  - ".java"
  - "maven"
  - "gradle"
role: coder
phase: analyze
language: java
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "README exists"
    command: "test -f README.md && echo 'OK' || echo 'MISSING'"
    expect_not: "MISSING"
---

> **Spec Override:** These patterns are DEFAULTS for when no spec is provided.
> If a project spec defines different architecture, package structure, or scope,
> FOLLOW THE SPEC. Do not apply these defaults over explicit spec requirements.
# Skill: Java Spring Boot

Spring Boot 3.x development — JPA, constructor injection, layered architecture.

Source: [Spring Boot Engineer](https://mcpmarket.com/es/tools/skills/spring-boot-engineer), [affaan-m/springboot-patterns](https://github.com/affaan-m/everything-claude-code/tree/main/skills/springboot-patterns) (70K stars).

---

## When to Use

- Building Spring Boot applications
- JPA/Hibernate data access
- REST API design with Spring
- Dependency injection patterns

---

## Core Rules

1. **Constructor injection** — never field injection (`@Autowired` on fields)
2. **Layered architecture** — Unless the project spec requires a different structure, use Controller → Service → Repository
3. **Records for DTOs** — immutable, concise
4. **Spring profiles** — `application-dev.yml`, `application-prod.yml`
5. **Validation annotations** — `@Valid`, `@NotNull`, `@Size`
6. **Exception handlers** — `@ControllerAdvice` for global error handling

---

## Patterns

### Constructor injection
```java
@Service
public class ItemService {
    private final ItemRepository repository;
    private final ClassificationService classifier;

    public ItemService(ItemRepository repository, ClassificationService classifier) {
        this.repository = repository;
        this.classifier = classifier;
    }
}
```

### JPA repository
```java
public interface ItemRepository extends JpaRepository<Item, String> {
    List<Item> findByStatusAndCategory(String status, String category);

    @Query("SELECT t FROM Item t WHERE t.createdAt > :since ORDER BY t.createdAt DESC")
    List<Item> findRecentItems(@Param("since") LocalDateTime since);
}
```

### Record DTO
```java
public record ItemResponse(
    String key,
    String summary,
    String status,
    @JsonFormat(pattern = "yyyy-MM-dd") LocalDate createdAt
) {}
```

### Global exception handler
```java
@ControllerAdvice
public class GlobalExceptionHandler {
    @ExceptionHandler(ItemNotFoundException.class)
    public ResponseEntity<ErrorResponse> handleNotFound(ItemNotFoundException ex) {
        return ResponseEntity.status(HttpStatus.NOT_FOUND)
            .body(new ErrorResponse(ex.getMessage()));
    }
}
```

---
name: csharp-patterns
description: "Idiomatic C# / .NET development — async/await, DI, LINQ, Entity Framework, NuGet."
triggers:
  - "csharp"
  - "c#"
  - ".cs"
  - "dotnet"
  - ".net"
  - "nuget"
  - "aspnet"
  - "blazor"
  - "entity framework"
role: coder
phase: analyze
language: csharp
mcp_tools:
  - "context7.query-docs"
verify:
  - name: "dotnet build"
    command: "dotnet build 2>&1 | tail -5"
    expect: "succeeded"
  - name: "dotnet test"
    command: "dotnet test 2>&1 | tail -5"
    expect: "Passed"
  - name: "nullable warnings"
    command: "dotnet build 2>&1 | grep -i 'CS8600\\|CS8601\\|CS8602\\|CS8603' | head -5 || echo 'OK'"
    expect: "OK"
  - name: "disposable resources"
    command: "grep -rn 'new.*Stream\\|new.*Connection\\|new.*Client' --include='*.cs' | grep -v 'using\\|await using\\|IDisposable\\|_test' | head -5 || echo 'OK'"
    expect: "OK"
    manual: "All IDisposable resources should be wrapped in using statements or await using for async disposables"
  - name: "README exists"
    command: "test -f README.md && echo 'OK' || echo 'MISSING'"
    expect_not: "MISSING"
---

> **Spec Override:** These patterns are defaults. If a project spec defines different
> architecture, package structure, or scope, follow the spec instead.
# Skill: C# / .NET Patterns

Idiomatic C# and .NET development — async/await, dependency injection, LINQ, Entity Framework, NuGet package management.

---

## When to Use

- Writing or reviewing C# / .NET code
- ASP.NET Core web APIs or Blazor apps
- Entity Framework database access
- .NET console applications or class libraries

---

## Core Rules

1. **Nullable reference types enabled** — `<Nullable>enable</Nullable>` in csproj
2. **async/await all the way** — no `.Result` or `.Wait()` blocking
3. **Constructor injection** — register in `Program.cs` or `Startup.cs`
4. **IDisposable via using** — `using var stream = ...` or `await using`
5. **LINQ over loops** — prefer declarative queries
6. **Record types for DTOs** — `record FooDto(string Name, int Count)`
7. **Result pattern for errors** — no exceptions for control flow

---

## Patterns

### Project Setup
```bash
dotnet new webapi -n MyApi
dotnet new console -n MyCli
dotnet new classlib -n MyLib
dotnet add package Newtonsoft.Json
```

### Dependency Injection
```csharp
// Program.cs (.NET 6+)
var builder = WebApplication.CreateBuilder(args);
builder.Services.AddScoped<IUserService, UserService>();
builder.Services.AddDbContext<AppDbContext>(options =>
    options.UseSqlite("Data Source=app.db"));

var app = builder.Build();
app.MapControllers();
app.Run();
```

### Async/Await
```csharp
// Always async all the way — never block with .Result
public async Task<User?> GetUserAsync(int id)
{
    return await _context.Users
        .Include(u => u.Orders)
        .FirstOrDefaultAsync(u => u.Id == id);
}
```

### LINQ
```csharp
var activeUsers = users
    .Where(u => u.IsActive)
    .OrderBy(u => u.Name)
    .Select(u => new UserDto(u.Name, u.Email))
    .ToList();
```

### Entity Framework Core
```csharp
public class AppDbContext : DbContext
{
    public DbSet<User> Users => Set<User>();
    public DbSet<Order> Orders => Set<Order>();

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        modelBuilder.Entity<User>()
            .HasMany(u => u.Orders)
            .WithOne(o => o.User);
    }
}
```

### Record Types for DTOs
```csharp
public record CreateUserRequest(string Name, string Email);
public record UserResponse(int Id, string Name, string Email, DateTime CreatedAt);
```

### Error Handling
```csharp
// Result pattern — no exceptions for expected failures
public record Result<T>
{
    public T? Value { get; init; }
    public string? Error { get; init; }
    public bool IsSuccess => Error is null;

    public static Result<T> Ok(T value) => new() { Value = value };
    public static Result<T> Fail(string error) => new() { Error = error };
}
```

### Configuration
```csharp
// appsettings.json binding
public class AppSettings
{
    public string ConnectionString { get; set; } = "";
    public int MaxRetries { get; set; } = 3;
}

builder.Services.Configure<AppSettings>(
    builder.Configuration.GetSection("App"));
```

---

## Anti-Patterns

- `.Result` or `.Wait()` — causes deadlocks in ASP.NET
- `async void` — only for event handlers, never for regular methods
- `catch (Exception)` with no rethrow — swallows errors silently
- `static` state in web apps — not thread-safe across requests
- Raw SQL strings — use parameterized queries or EF
- `new DbContext()` manually — always inject via DI

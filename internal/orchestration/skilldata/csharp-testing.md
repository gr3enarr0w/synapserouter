---
name: csharp-testing
description: "C# testing — xUnit, NUnit, Moq, FluentAssertions, WebApplicationFactory."
triggers:
  - "xunit"
  - "nunit"
  - "csharp+test"
  - "dotnet test"
  - "moq"
  - "c#+test"
  - "mstest"
role: tester
phase: verify
language: csharp
verify:
  - name: "dotnet test"
    command: "dotnet test 2>&1 | tail -10"
    expect: "Passed"
---
# csharp-testing

## xUnit (preferred)
- [Fact] for single tests, [Theory] for parameterized
- [InlineData], [MemberData], [ClassData] for test data
- IClassFixture<T> for shared context
- IAsyncLifetime for async setup/teardown

## NUnit
- [Test], [TestCase], [TestFixture]
- [SetUp], [TearDown], [OneTimeSetUp]
- Assert.That() with constraint model

## Moq
- Mock<IService>() for interface mocking
- Setup(x => x.Method()).Returns(value)
- Verify(x => x.Method(), Times.Once)
- It.IsAny<T>() for argument matching

## FluentAssertions
- actual.Should().Be(expected)
- collection.Should().HaveCount(3).And.Contain(item)
- action.Should().Throw<Exception>()

## Integration Testing
- WebApplicationFactory<TStartup> for ASP.NET Core
- TestServer for in-memory HTTP server
- InMemoryDatabase for EF Core (use Testcontainers for real DB)
- Bogus for fake data generation

## Anti-Patterns
- Static test dependencies (use DI)
- Missing IDisposable cleanup
- Testing EF against real DB without isolation
- Not using async test patterns for async code

---
name: kotlin-testing
description: "Kotlin testing — JUnit5, MockK, Kotest, coroutine testing, Turbine."
triggers:
  - "kotlin+test"
  - "junit+kotlin"
  - "mockk"
  - "kotest"
  - "kotlin+spec"
role: tester
phase: verify
language: kotlin
verify:
  - name: "gradle test"
    command: "gradle test 2>&1 || ./gradlew test 2>&1 || echo 'TEST_FAILED'"
    expect_not: "TEST_FAILED"
---
# kotlin-testing

## JUnit5 + Kotlin
- @Test, @BeforeEach, @AfterEach, @Nested for grouping
- @ParameterizedTest with @ValueSource, @CsvSource
- assertThrows<ExceptionType> { }
- Kotlin-specific: shouldBe, shouldNotBe (Kotest matchers)

## MockK
- every { mock.method() } returns value
- verify { mock.method() }
- slot<Type>() for argument capture
- coEvery/coVerify for suspend functions
- unmockkAll() in @AfterEach

## Coroutine Testing
- runTest { } (not runBlocking)
- TestDispatcher for time control
- advanceUntilIdle(), advanceTimeBy()
- Turbine for Flow testing: flow.test { awaitItem() }

## Anti-Patterns
- runBlocking in tests (use runTest)
- Missing unmockkAll() cleanup
- Testing coroutines without TestDispatcher
- No assertions in test methods

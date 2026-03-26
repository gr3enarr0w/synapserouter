---
name: java-testing
description: "Java testing — JUnit5, Mockito, AssertJ, Testcontainers, Spring Boot test slices."
triggers:
  - "junit"
  - "java+test"
  - "mockito"
  - "assertj"
  - "testcontainers"
  - "java+spec"
role: tester
phase: verify
language: java
verify:
  - name: "run tests"
    command: "mvn test 2>&1 || gradle test 2>&1 || echo 'TEST_FAILED'"
    expect_not: "TEST_FAILED"
---
# java-testing

## JUnit5
- @Test, @BeforeEach, @AfterEach lifecycle
- @ParameterizedTest with @ValueSource, @CsvSource, @MethodSource
- @Nested for test grouping
- @DisplayName for readable names
- assertThrows(Exception.class, () -> {})
- assertAll() for grouped assertions

## Mockito
- @Mock, @InjectMocks with @ExtendWith(MockitoExtension.class)
- when(mock.method()).thenReturn(value)
- verify(mock, times(1)).method()
- ArgumentCaptor for capturing arguments
- doThrow/doAnswer for void methods

## AssertJ (preferred over JUnit assertions)
- assertThat(actual).isEqualTo(expected)
- assertThat(list).hasSize(3).contains("item")
- assertThatThrownBy(() -> {}).isInstanceOf(Exception.class)

## Integration Testing
- Testcontainers for real DB/Redis/Kafka
- @SpringBootTest for full context
- @WebMvcTest for controller slice
- @DataJpaTest for repository slice
- TestRestTemplate / WebTestClient

## Anti-Patterns
- Testing private methods directly
- @Autowired in tests (use constructor injection)
- No assertions in test methods
- Thread.sleep for async testing (use Awaitility)
- Testing with in-memory DB when prod uses different engine

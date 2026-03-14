package main

import (
	"encoding/json"
	"testing"
)

// TestResponsesAPIChaining tests the fix for BUG-RESPONSES-CHAIN-001
// Verifies that previous_response_id properly reconstructs conversation context
func TestResponsesAPIChaining(t *testing.T) {
	db = newCompatTestDB(t)

	// Simulate first response (seed turn)
	firstResponseID := "resp-001"
	firstInput := json.RawMessage(`"Remember this secret code: ALPHA-BRAVO-123"`)
	firstPayload := map[string]interface{}{
		"id":          firstResponseID,
		"object":      "response",
		"session_id":  "test-session-chain",
		"input":       firstInput,
		"output_text": "I'll remember that code.",
		"model":       "test-model",
	}
	storeResponsePayload(firstResponseID, firstPayload)

	// Simulate second response (follow-up, chained to first)
	secondResponseID := "resp-002"
	secondInput := json.RawMessage(`"What was the code again?"`)
	secondPayload := map[string]interface{}{
		"id":                   secondResponseID,
		"object":               "response",
		"session_id":           "test-session-chain",
		"input":                secondInput,
		"output_text":          "The code is ALPHA-BRAVO-123",
		"previous_response_id": firstResponseID,
		"model":                "test-model",
	}
	storeResponsePayload(secondResponseID, secondPayload)

	// Simulate third response (chained to second, which chains to first)
	thirdResponseID := "resp-003"
	thirdInput := json.RawMessage(`"Can you repeat it one more time?"`)
	thirdPayload := map[string]interface{}{
		"id":                   thirdResponseID,
		"object":               "response",
		"session_id":           "test-session-chain",
		"input":                thirdInput,
		"output_text":          "Yes, the code is ALPHA-BRAVO-123",
		"previous_response_id": secondResponseID,
		"model":                "test-model",
	}
	storeResponsePayload(thirdResponseID, thirdPayload)

	// Test reconstruction from third response (should get all prior messages)
	reconstructed := reconstructConversationChain(thirdResponseID)

	// Expected messages (chronological order):
	// 1. User: "Remember this secret code: ALPHA-BRAVO-123"
	// 2. Assistant: "I'll remember that code."
	// 3. User: "What was the code again?"
	// 4. Assistant: "The code is ALPHA-BRAVO-123"
	// 5. User: "Can you repeat it one more time?" (from current request)
	// 6. Assistant: "Yes, the code is ALPHA-BRAVO-123" (from current request)

	if len(reconstructed) < 4 {
		t.Errorf("Expected at least 4 messages from chain, got %d", len(reconstructed))
		for i, msg := range reconstructed {
			t.Logf("  Message %d: [%s] %s", i, msg.Role, msg.Content)
		}
		t.Fatal("Insufficient messages reconstructed")
	}

	// Verify first message (from first turn)
	if reconstructed[0].Role != "user" {
		t.Errorf("Expected first message role 'user', got '%s'", reconstructed[0].Role)
	}
	if reconstructed[0].Content != "Remember this secret code: ALPHA-BRAVO-123" {
		t.Errorf("Expected first message content to contain secret code, got: %s", reconstructed[0].Content)
	}

	// Verify second message (assistant response from first turn)
	if reconstructed[1].Role != "assistant" {
		t.Errorf("Expected second message role 'assistant', got '%s'", reconstructed[1].Role)
	}

	// Verify third message (user from second turn)
	if reconstructed[2].Role != "user" {
		t.Errorf("Expected third message role 'user', got '%s'", reconstructed[2].Role)
	}
	if reconstructed[2].Content != "What was the code again?" {
		t.Errorf("Expected third message to be follow-up question, got: %s", reconstructed[2].Content)
	}

	// Verify fourth message (assistant response from second turn)
	if reconstructed[3].Role != "assistant" {
		t.Errorf("Expected fourth message role 'assistant', got '%s'", reconstructed[3].Role)
	}

	t.Logf("✅ Response chaining test passed - reconstructed %d messages from chain", len(reconstructed))
	t.Logf("   Full conversation history:")
	for i, msg := range reconstructed {
		t.Logf("     %d. [%s] %s", i+1, msg.Role, msg.Content)
	}
}

// TestResponsesAPIChainingWithStructuredInput tests chaining with structured message arrays
func TestResponsesAPIChainingWithStructuredInput(t *testing.T) {
	db = newCompatTestDB(t)

	// Simulate response with structured input (array of messages)
	responseID := "resp-structured-001"
	structuredInput := json.RawMessage(`[
		{"role":"user","content":"Hello"},
		{"role":"assistant","content":"Hi there!"},
		{"role":"user","content":"How are you?"}
	]`)

	payload := map[string]interface{}{
		"id":          responseID,
		"object":      "response",
		"session_id":  "test-structured",
		"input":       structuredInput,
		"output_text": "I'm doing well, thank you!",
		"model":       "test-model",
	}
	storeResponsePayload(responseID, payload)

	// Reconstruct
	reconstructed := reconstructConversationChain(responseID)

	// Should have 3 messages from input + 1 from output = 4 total
	if len(reconstructed) != 4 {
		t.Errorf("Expected 4 messages (3 from input + 1 from output), got %d", len(reconstructed))
	}

	// Verify message order
	expectedRoles := []string{"user", "assistant", "user", "assistant"}
	for i, expectedRole := range expectedRoles {
		if i >= len(reconstructed) {
			t.Errorf("Missing message at index %d", i)
			continue
		}
		if reconstructed[i].Role != expectedRole {
			t.Errorf("Message %d: expected role '%s', got '%s'", i, expectedRole, reconstructed[i].Role)
		}
	}

	t.Logf("✅ Structured input chaining test passed")
}

// TestResponsesAPICycleDetection tests that cycles in the chain are detected
func TestResponsesAPICycleDetection(t *testing.T) {
	db = newCompatTestDB(t)

	// Create a cycle: resp-a -> resp-b -> resp-c -> resp-a
	payloadA := map[string]interface{}{
		"id":                   "resp-cycle-a",
		"input":                json.RawMessage(`"Message A"`),
		"output_text":          "Response A",
		"previous_response_id": "resp-cycle-c",
	}
	payloadB := map[string]interface{}{
		"id":                   "resp-cycle-b",
		"input":                json.RawMessage(`"Message B"`),
		"output_text":          "Response B",
		"previous_response_id": "resp-cycle-a",
	}
	payloadC := map[string]interface{}{
		"id":                   "resp-cycle-c",
		"input":                json.RawMessage(`"Message C"`),
		"output_text":          "Response C",
		"previous_response_id": "resp-cycle-b",
	}

	storeResponsePayload("resp-cycle-a", payloadA)
	storeResponsePayload("resp-cycle-b", payloadB)
	storeResponsePayload("resp-cycle-c", payloadC)

	// Reconstruct from C (should detect cycle and stop)
	reconstructed := reconstructConversationChain("resp-cycle-c")

	// Should stop when cycle is detected, not infinite loop
	if len(reconstructed) == 0 {
		t.Error("Expected some messages before cycle detection")
	}

	// Should not have more than 3 turns worth of messages (6 total)
	if len(reconstructed) > 6 {
		t.Errorf("Cycle detection failed - got %d messages (expected <=6)", len(reconstructed))
	}

	t.Logf("✅ Cycle detection test passed - stopped at %d messages", len(reconstructed))
}

// TestReponseSessionID tests the helper function
func TestResponseSessionID(t *testing.T) {
	db = newCompatTestDB(t)

	// Store a response with a session ID
	testID := "resp-session-test"
	testSessionID := "my-test-session-123"
	payload := map[string]interface{}{
		"id":         testID,
		"session_id": testSessionID,
	}
	storeResponsePayload(testID, payload)

	// Retrieve session ID
	retrievedSessionID := responseSessionID(testID)
	if retrievedSessionID != testSessionID {
		t.Errorf("Expected session ID '%s', got '%s'", testSessionID, retrievedSessionID)
	}

	// Test with non-existent response
	nonExistentSessionID := responseSessionID("does-not-exist")
	if nonExistentSessionID != "" {
		t.Errorf("Expected empty string for non-existent response, got '%s'", nonExistentSessionID)
	}

	t.Logf("✅ responseSessionID test passed")
}

package agent

import (
    "fmt"
    "encoding/json"
    "os"
    "testing"
)

type intentTestCase struct {
    Message  string `json:"message"`
    Expected string `json:"expected"`
}

func TestIntentRouterAccuracy(t *testing.T) {
    data, err := os.ReadFile("../../tests/functional/intent_test_cases.json")
    if err != nil {
        t.Skipf("Test fixture not found: %v", err)
    }

    var cases []intentTestCase
    if err := json.Unmarshal(data, &cases); err != nil {
        t.Fatalf("Failed to parse test cases: %v", err)
    }

    router := NewIntentRouter()
    pass, fail := 0, 0
    failures := []string{}

    for _, tc := range cases {
        got := string(router.Classify(tc.Message))
        if got == tc.Expected {
            pass++
        } else {
            fail++
            failures = append(failures, fmt.Sprintf("got=%s want=%s | %s", got, tc.Expected, tc.Message))
        }
    }

    accuracy := float64(pass) / float64(pass+fail) * 100
    t.Logf("Accuracy: %.1f%% (%d/%d pass, %d fail)", accuracy, pass, pass+fail, fail)
    
    if len(failures) > 0 && len(failures) <= 50 {
        for _, f := range failures {
            t.Logf("  FAIL: %s", f)
        }
    } else if len(failures) > 50 {
        for _, f := range failures[:50] {
            t.Logf("  FAIL: %s", f)
        }
        t.Logf("  ... and %d more failures", len(failures)-50)
    }

    if accuracy < 90.0 {
        t.Errorf("Accuracy %.1f%% is below 90%% threshold", accuracy)
    }
}

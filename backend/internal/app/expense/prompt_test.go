package expense

import (
	"strings"
	"testing"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

func TestSystemPromptFillsTemplate(t *testing.T) {
	prompt, err := systemPrompt(time.Date(2026, 11, 3, 23, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("systemPrompt: %v", err)
	}
	if !strings.Contains(prompt, "Сегодня 2026-11-03, вторник.") {
		t.Fatalf("prompt has no date:\n%s", prompt)
	}
	for _, category := range domain.Categories() {
		if !strings.Contains(prompt, string(category)) {
			t.Fatalf("prompt has no category %q", category)
		}
	}
	for _, name := range []string{toolAdd, toolList} {
		if !strings.Contains(prompt, name) {
			t.Fatalf("prompt does not mention %s", name)
		}
	}
	if strings.Contains(prompt, "{{") || strings.Contains(prompt, "<no value>") {
		t.Fatalf("prompt has unfilled placeholders:\n%s", prompt)
	}
}

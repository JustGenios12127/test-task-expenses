package expense

import (
	"testing"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

func TestClarifySecondExpense(t *testing.T) {
	intent := clarifyTarget("Удали вторую траут", domain.Intent{Action: domain.ActionDelete, Match: "траут"})
	if intent.Index != 2 || intent.Match != "" {
		t.Fatalf("intent = %+v", intent)
	}
}

func TestClarifySecondCoffeeAmount(t *testing.T) {
	intent := clarifyTarget("Удали вторую трату Кофе 450", domain.Intent{Action: domain.ActionDelete, Match: "Удали вторую трату Кофе 450"})
	if intent.Index != 2 || intent.Amount != 45000 || intent.Match != "Кофе" {
		t.Fatalf("intent = %+v", intent)
	}
}

func TestClarifyCopiedListLine(t *testing.T) {
	intent := clarifyTarget("Удали трату 30.10.2026 — Кофе, 450 ₽, еда", domain.Intent{Action: domain.ActionDelete, Match: "трату"})
	if intent.Amount != 45000 || intent.Match != "Кофе" || intent.Category != domain.CategoryFood {
		t.Fatalf("intent = %+v", intent)
	}
	if !intent.SpentAt.Equal(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("spent = %s", intent.SpentAt)
	}
}

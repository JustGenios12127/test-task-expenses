package groq

import (
	"errors"
	"strings"
	"testing"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

func TestIntentFromModel(t *testing.T) {
	today := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)

	add, err := intentFromModel(`{"action":"add","amount":1200,"category":"транспорт","spent_at":"2026-10-29","comment":"такси"}`, today)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if add.Action != domain.ActionAdd || add.Amount != 120000 || add.Category != domain.CategoryTransport || add.Comment != "такси" {
		t.Fatalf("add = %+v", add)
	}
	if !add.SpentAt.Equal(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("spent at = %s", add.SpentAt)
	}

	list, err := intentFromModel(`{"action":"list","category":"еда","period":"month"}`, today)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list.Action != domain.ActionList || list.Category != domain.CategoryFood || list.Period != domain.PeriodMonth {
		t.Fatalf("list = %+v", list)
	}

	week, err := intentFromModel(`{"action":"show","period":"неделю"}`, today)
	if err != nil {
		t.Fatalf("week: %v", err)
	}
	if week.Action != domain.ActionList || week.Period != domain.PeriodWeek {
		t.Fatalf("week = %+v", week)
	}

	update, err := intentFromModel(`{"action":"update","amount":"350","new_amount":450}`, today)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if update.Action != domain.ActionUpdate || update.Amount != 35000 || update.NewAmount != 45000 {
		t.Fatalf("update = %+v", update)
	}

	remove, err := intentFromModel(`{"action":"delete","match":"такси"}`, today)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if remove.Action != domain.ActionDelete || remove.Match != "такси" {
		t.Fatalf("delete = %+v", remove)
	}

	products, err := intentFromModel(`{"action":"list","category":"продукты","from":"2026-10-01","to":"2026-10-30","period":"month"}`, today)
	if err != nil {
		t.Fatalf("products: %v", err)
	}
	if products.Category != domain.CategoryFood || products.Period != domain.PeriodMonth || products.From.Day() != 1 || products.To.Day() != 30 {
		t.Fatalf("products = %+v", products)
	}

	latest, err := intentFromModel(`{"action":"убери","latest":true}`, today)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.Action != domain.ActionDelete || !latest.Latest {
		t.Fatalf("latest = %+v", latest)
	}

	moved, err := intentFromModel(`{"action":"update","match":"такси","new_spent_at":"2026-10-29"}`, today)
	if err != nil {
		t.Fatalf("moved: %v", err)
	}
	if moved.Action != domain.ActionUpdate || moved.Match != "такси" || !moved.NewSpentAt.Equal(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("moved = %+v", moved)
	}
}

func TestUserContentKeepsRecentDialogue(t *testing.T) {
	text := userContent("а за еду?", []domain.Turn{
		{Role: domain.RoleUser, Text: "покажи траты за неделю"},
		{Role: domain.RoleAssistant, Text: "За неделю: 1200 ₽"},
	})
	if !strings.Contains(text, "Диалог:") || !strings.Contains(text, "Пользователь: покажи траты за неделю") || !strings.Contains(text, "Новое сообщение: а за еду?") {
		t.Fatalf("content = %q", text)
	}
}

func TestFillAddCommentUsesPurchaseName(t *testing.T) {
	today := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	intent := fillAddComment(domain.Intent{
		Action:   domain.ActionAdd,
		Amount:   120000,
		Category: domain.CategoryTransport,
		SpentAt:  time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC),
	}, "такси 1200 вчера", today)
	if intent.Comment != "такси" {
		t.Fatalf("comment = %q", intent.Comment)
	}
}

func TestIntentFromModelRejectsNonExpense(t *testing.T) {
	today := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	for _, raw := range []string{
		`{"error":"not_expense"}`,
		`{"action":"unknown"}`,
		`{"action":"unknown","amount":350}`,
	} {
		_, err := intentFromModel(raw, today)
		if !errors.Is(err, domain.ErrNotExpense) {
			t.Fatalf("%s error = %v", raw, err)
		}
	}
}

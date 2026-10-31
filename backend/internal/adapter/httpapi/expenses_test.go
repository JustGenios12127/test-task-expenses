package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"test-task-expenses/backend/internal/app/family"
	domain "test-task-expenses/backend/internal/domain/expense"
)

func TestListReturnsStoredNames(t *testing.T) {
	spent := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	handler := New(&fakePanel{items: []family.Expense{{
		ID: 7, UserID: 42, Name: "@ivan", Amount: 35050,
		Category: domain.CategoryFood, SpentAt: spent, Comment: "кофе",
		CreatedAt: spent, UpdatedAt: spent,
	}}})

	req := httptest.NewRequest(http.MethodGet, "/api/expenses", nil)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Expenses []expenseJSON `json:"expenses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Expenses) != 1 || payload.Expenses[0].Name != "@ivan" || payload.Expenses[0].Amount != "350.50" {
		t.Fatalf("payload = %+v", payload.Expenses)
	}
}

func TestCreateReturnsSavedExpense(t *testing.T) {
	spent := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	panel := &fakePanel{created: family.Expense{
		ID: 3, UserID: 10, Name: "@ivan", Amount: 1000,
		Category: domain.CategoryOther, SpentAt: spent, Comment: "проверка",
		CreatedAt: spent, UpdatedAt: spent,
	}}
	handler := New(panel)
	req := httptest.NewRequest(http.MethodPost, "/api/expenses", strings.NewReader(`{"userId":10,"amount":"10","category":"другое","spentAt":"2026-10-30","comment":"проверка"}`))
	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if panel.input.UserID != 10 || panel.input.Amount != "10" || panel.input.Comment != "проверка" {
		t.Fatalf("input = %+v", panel.input)
	}
}

func TestDeleteMissingExpense(t *testing.T) {
	handler := New(&fakePanel{deleteErr: domain.ErrNotFound})
	req := httptest.NewRequest(http.MethodDelete, "/api/expenses/4", nil)
	req.SetPathValue("id", "4")
	rec := httptest.NewRecorder()
	handler.Delete(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestListReadsFiltersAndTotal(t *testing.T) {
	panel := &fakePanel{items: []family.Expense{{ID: 1, Amount: 35000}, {ID: 2, Amount: 15050}}}
	req := httptest.NewRequest(http.MethodGet, "/api/expenses?userId=42&category=%D0%B5%D0%B4%D0%B0&from=2026-10-01&to=2026-10-31", nil)
	rec := httptest.NewRecorder()
	New(panel).List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := family.Query{UserID: 42, Category: "еда", From: "2026-10-01", To: "2026-10-31"}
	if panel.query != want {
		t.Fatalf("query = %+v", panel.query)
	}
	var payload struct {
		Total string `json:"total"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Total != "500.50" || payload.Count != 2 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestListRejectsBadMember(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/expenses?userId=abc", nil)
	rec := httptest.NewRecorder()
	New(&fakePanel{}).List(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateReportsConflict(t *testing.T) {
	panel := &fakePanel{updateErr: domain.ErrConflict}
	req := httptest.NewRequest(http.MethodPut, "/api/expenses/7", strings.NewReader(`{"userId":10,"amount":"10","category":"еда","spentAt":"2026-10-30","updatedAt":"2026-10-30T10:00:00.5Z"}`))
	req.SetPathValue("id", "7")
	rec := httptest.NewRecorder()
	New(panel).Update(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if panel.input.Version != "2026-10-30T10:00:00.5Z" {
		t.Fatalf("version = %q", panel.input.Version)
	}
}

type fakePanel struct {
	items     []family.Expense
	created   family.Expense
	input     family.Input
	query     family.Query
	updateErr error
	deleteErr error
}

func (f *fakePanel) List(_ context.Context, query family.Query) (family.Page, error) {
	f.query = query
	var total int64
	for _, item := range f.items {
		total += item.Amount
	}
	return family.Page{Items: f.items, Total: total, Count: len(f.items)}, nil
}

func (f *fakePanel) Members(context.Context) ([]family.Member, error) {
	return []family.Member{{ID: 10, Name: "@ivan"}}, nil
}

func (f *fakePanel) Create(_ context.Context, input family.Input) (family.Expense, error) {
	f.input = input
	return f.created, nil
}

func (f *fakePanel) Update(_ context.Context, _ int64, input family.Input) (family.Expense, error) {
	f.input = input
	return family.Expense{}, f.updateErr
}

func (f *fakePanel) Delete(context.Context, int64) error {
	return f.deleteErr
}

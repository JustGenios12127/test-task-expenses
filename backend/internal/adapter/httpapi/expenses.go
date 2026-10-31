package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"test-task-expenses/backend/internal/app/family"
	domain "test-task-expenses/backend/internal/domain/expense"
)

type Panel interface {
	List(ctx context.Context, query family.Query) (family.Page, error)
	Members(ctx context.Context) ([]family.Member, error)
	Create(ctx context.Context, input family.Input) (family.Expense, error)
	Update(ctx context.Context, id int64, input family.Input) (family.Expense, error)
	Delete(ctx context.Context, id int64) error
}

type Handler struct {
	panel Panel
}

func New(panel Panel) *Handler {
	return &Handler{panel: panel}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	query := family.Query{
		Category: params.Get("category"),
		From:     params.Get("from"),
		To:       params.Get("to"),
	}
	if raw := params.Get("userId"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, errorBody("Проверьте фильтры"))
			return
		}
		query.UserID = id
	}
	page, err := h.panel.List(r.Context(), query)
	if errors.Is(err, family.ErrInvalidFilter) {
		writeJSON(w, http.StatusBadRequest, errorBody("Проверьте фильтры"))
		return
	}
	if err != nil {
		slog.Error("list family expenses", "err", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("не удалось загрузить траты"))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Expenses []expenseJSON `json:"expenses"`
		Total    string        `json:"total"`
		Count    int           `json:"count"`
	}{Expenses: expensesJSON(page.Items), Total: formatAmount(page.Total), Count: page.Count})
}

func (h *Handler) Members(w http.ResponseWriter, r *http.Request) {
	members, err := h.panel.Members(r.Context())
	if err != nil {
		slog.Error("list family members", "err", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("не удалось загрузить членов семьи"))
		return
	}
	payload := struct {
		Members []memberJSON `json:"members"`
	}{Members: make([]memberJSON, 0, len(members))}
	for _, member := range members {
		payload.Members = append(payload.Members, memberJSON{ID: member.ID, Name: member.Name})
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	input, err := readInput(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("Проверьте поля траты"))
		return
	}
	item, err := h.panel.Create(r.Context(), input)
	if err != nil {
		writePanelError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, expenseBody(item))
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("Трата не найдена"))
		return
	}
	input, err := readInput(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("Проверьте поля траты"))
		return
	}
	item, err := h.panel.Update(r.Context(), id, input)
	if err != nil {
		writePanelError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, expenseBody(item))
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("Трата не найдена"))
		return
	}
	if err := h.panel.Delete(r.Context(), id); err != nil {
		writePanelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type expenseJSON struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"userId"`
	Name      string `json:"name"`
	Amount    string `json:"amount"`
	Category  string `json:"category"`
	SpentAt   string `json:"spentAt"`
	Comment   string `json:"comment"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type memberJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type expenseRequest struct {
	UserID   int64  `json:"userId"`
	Amount   string `json:"amount"`
	Category string `json:"category"`
	SpentAt   string `json:"spentAt"`
	Comment   string `json:"comment"`
	UpdatedAt string `json:"updatedAt"`
}

func readInput(w http.ResponseWriter, r *http.Request) (family.Input, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	defer r.Body.Close()

	var body expenseRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&body); err != nil {
		return family.Input{}, err
	}
	return family.Input{
		UserID:   body.UserID,
		Amount:   body.Amount,
		Category: body.Category,
		SpentAt:  body.SpentAt,
		Comment:  body.Comment,
		Version:  body.UpdatedAt,
	}, nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrNotFound
	}
	return id, nil
}

func expensesJSON(items []family.Expense) []expenseJSON {
	payload := make([]expenseJSON, 0, len(items))
	for _, item := range items {
		payload = append(payload, toExpenseJSON(item))
	}
	return payload
}

func expenseBody(item family.Expense) struct {
	Expense expenseJSON `json:"expense"`
} {
	return struct {
		Expense expenseJSON `json:"expense"`
	}{Expense: toExpenseJSON(item)}
}

func toExpenseJSON(item family.Expense) expenseJSON {
	return expenseJSON{
		ID:        item.ID,
		UserID:    item.UserID,
		Name:      item.Name,
		Amount:    formatAmount(item.Amount),
		Category:  string(item.Category),
		SpentAt:   item.SpentAt.Format("2006-01-02"),
		Comment:   item.Comment,
		CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func writePanelError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody("Трата не найдена"))
	case errors.Is(err, domain.ErrConflict), errors.Is(err, family.ErrInvalidVersion):
		writeJSON(w, http.StatusConflict, errorBody("Трату уже изменили, список обновлён. Проверьте данные и сохраните ещё раз"))
	case errors.Is(err, family.ErrUnknownMember):
		writeJSON(w, http.StatusBadRequest, errorBody("Выберите члена семьи"))
	case errors.Is(err, domain.ErrInvalidAmount):
		writeJSON(w, http.StatusBadRequest, errorBody("Укажите сумму больше нуля"))
	case errors.Is(err, domain.ErrInvalidCategory):
		writeJSON(w, http.StatusBadRequest, errorBody("Выберите категорию"))
	case errors.Is(err, domain.ErrFutureDate):
		writeJSON(w, http.StatusBadRequest, errorBody("Дата не может быть в будущем"))
	case errors.Is(err, domain.ErrInvalidDate), errors.Is(err, domain.ErrUserRequired):
		writeJSON(w, http.StatusBadRequest, errorBody("Проверьте дату и члена семьи"))
	default:
		slog.Error("change family expense", "err", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("Не удалось сохранить трату"))
	}
}

func errorBody(message string) map[string]string {
	return map[string]string{"error": message}
}

func formatAmount(kopecks int64) string {
	return fmt.Sprintf("%d.%02d", kopecks/100, kopecks%100)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil && !errors.Is(err, io.EOF) {
		slog.Error("write json", "err", err)
	}
}

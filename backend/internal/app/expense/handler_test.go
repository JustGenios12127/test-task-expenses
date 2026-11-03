package expense

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"test-task-expenses/backend/internal/domain/chat"
	domain "test-task-expenses/backend/internal/domain/expense"
)

var today = time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC)

type scriptedModel struct {
	steps []chat.Message
	err   error
	calls [][]chat.Message
}

func (m *scriptedModel) Complete(_ context.Context, system string, messages []chat.Message, tools []chat.Tool) (chat.Message, error) {
	if !strings.Contains(system, "2026-10-30") || len(tools) != 4 {
		return chat.Message{}, errors.New("unexpected request")
	}
	m.calls = append(m.calls, append([]chat.Message(nil), messages...))
	if m.err != nil {
		return chat.Message{}, m.err
	}
	if len(m.steps) == 0 {
		return chat.Message{Role: chat.RoleAssistant, ToolCalls: []chat.ToolCall{{ID: "loop", Name: toolList, Arguments: "{}"}}}, nil
	}
	step := m.steps[0]
	m.steps = m.steps[1:]
	return step, nil
}

func (m *scriptedModel) toolResult(t *testing.T, call int) string {
	t.Helper()
	messages := m.calls[call]
	last := messages[len(messages)-1]
	if last.Role != chat.RoleTool {
		t.Fatalf("message %d is %+v, want tool result", call, last)
	}
	return last.Text
}

func toolCall(id, name, args string) chat.Message {
	return chat.Message{Role: chat.RoleAssistant, ToolCalls: []chat.ToolCall{{ID: id, Name: name, Arguments: args}}}
}

func reply(text string) chat.Message {
	return chat.Message{Role: chat.RoleAssistant, Text: text}
}

type memoryStore struct {
	items     []domain.Expense
	nextID    int64
	lastQuery domain.Query
	err       error
}

func (s *memoryStore) Add(_ context.Context, item domain.Expense) (domain.Expense, error) {
	if s.err != nil {
		return domain.Expense{}, s.err
	}
	s.nextID++
	item.ID = s.nextID
	s.items = append(s.items, item)
	return item, nil
}

func (s *memoryStore) List(_ context.Context, query domain.Query) ([]domain.Expense, error) {
	s.lastQuery = query
	var out []domain.Expense
	for i := len(s.items) - 1; i >= 0; i-- {
		item := s.items[i]
		if item.UserID != query.UserID ||
			(query.Match != "" && !strings.Contains(strings.ToLower(item.Comment), strings.ToLower(query.Match))) ||
			(query.Category != "" && item.Category != query.Category) ||
			(query.Amount > 0 && item.Amount != query.Amount) ||
			(!query.From.IsZero() && item.SpentAt.Before(query.From)) ||
			(!query.To.IsZero() && item.SpentAt.After(query.To)) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *memoryStore) Total(ctx context.Context, query domain.Query) (int64, int, error) {
	items, _ := s.List(ctx, query)
	var total int64
	for _, item := range items {
		total += item.Amount
	}
	return total, len(items), nil
}

func (s *memoryStore) Get(_ context.Context, userID, id int64) (domain.Expense, error) {
	for _, item := range s.items {
		if item.ID == id && item.UserID == userID {
			return item, nil
		}
	}
	return domain.Expense{}, domain.ErrNotFound
}

func (s *memoryStore) Update(_ context.Context, userID, id int64, patch domain.Patch) (domain.Expense, error) {
	for i, item := range s.items {
		if item.ID == id && item.UserID == userID {
			if patch.Amount > 0 {
				item.Amount = patch.Amount
			}
			if patch.Category != "" {
				item.Category = patch.Category
			}
			if !patch.SpentAt.IsZero() {
				item.SpentAt = patch.SpentAt
			}
			if patch.SetComment {
				item.Comment = patch.Comment
			}
			s.items[i] = item
			return item, nil
		}
	}
	return domain.Expense{}, domain.ErrNotFound
}

func (s *memoryStore) Delete(_ context.Context, userID, id int64) (domain.Expense, error) {
	for i, item := range s.items {
		if item.ID == id && item.UserID == userID {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return item, nil
		}
	}
	return domain.Expense{}, domain.ErrNotFound
}

type fakeSender struct {
	chatID int64
	texts  []string
}

func (f *fakeSender) SendText(_ context.Context, chatID int64, text string) error {
	f.chatID = chatID
	f.texts = append(f.texts, text)
	return nil
}

func (f *fakeSender) last() string {
	if len(f.texts) == 0 {
		return ""
	}
	return f.texts[len(f.texts)-1]
}

func newTestHandler(model Model, store Store, sender Sender) *Handler {
	handler := NewHandler(model, store, sender, time.UTC)
	handler.now = func() time.Time { return today }
	return handler
}

func seed(userID int64, amount int64, category domain.Category, comment string) domain.Expense {
	return domain.Expense{UserID: userID, Amount: amount, Category: category, SpentAt: domain.DateUTC(today), Comment: comment}
}

func TestAddRunsToolAndSendsModelReply(t *testing.T) {
	model := &scriptedModel{steps: []chat.Message{
		toolCall("c1", toolAdd, `{"amount":350,"category":"еда","comment":"кофе"}`),
		reply("Записал: кофе, 350 ₽."),
	}}
	store := &memoryStore{}
	sender := &fakeSender{}

	if err := newTestHandler(model, store, sender).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "кофе 350"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(store.items) != 1 {
		t.Fatalf("items = %+v", store.items)
	}
	saved := store.items[0]
	if saved.UserID != 55 || saved.Amount != 35000 || saved.Category != domain.CategoryFood || !saved.SpentAt.Equal(domain.DateUTC(today)) {
		t.Fatalf("saved = %+v", saved)
	}
	if !strings.Contains(model.toolResult(t, 1), `"added"`) {
		t.Fatalf("tool result = %s", model.toolResult(t, 1))
	}
	if sender.chatID != 77 || sender.last() != "Записал: кофе, 350 ₽." {
		t.Fatalf("sent %d %q", sender.chatID, sender.last())
	}
}

func TestDeleteFindsOwnExpenseByList(t *testing.T) {
	store := &memoryStore{}
	own, _ := store.Add(context.Background(), seed(55, 120000, domain.CategoryTransport, "такси"))
	foreign, _ := store.Add(context.Background(), seed(66, 50000, domain.CategoryTransport, "такси"))
	model := &scriptedModel{steps: []chat.Message{
		toolCall("c1", toolList, `{"search":"такси"}`),
		toolCall("c2", toolDelete, `{"id":1}`),
		reply("Удалил такси."),
	}}

	if err := newTestHandler(model, store, &fakeSender{}).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "удали такси"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	found := model.toolResult(t, 1)
	if !strings.Contains(found, `"id":1`) || strings.Contains(found, `"id":2`) {
		t.Fatalf("list result = %s", found)
	}
	if _, err := store.Get(context.Background(), 55, own.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("own expense still exists: %v", err)
	}
	if _, err := store.Get(context.Background(), 66, foreign.ID); err != nil {
		t.Fatalf("foreign expense was touched: %v", err)
	}
}

func TestToolsCannotTouchForeignExpense(t *testing.T) {
	store := &memoryStore{}
	foreign, _ := store.Add(context.Background(), seed(66, 50000, domain.CategoryTransport, "такси"))
	model := &scriptedModel{steps: []chat.Message{
		toolCall("c1", toolDelete, `{"id":1}`),
		toolCall("c2", toolUpdate, `{"id":1,"amount":1}`),
		reply("Не нашёл такую трату."),
	}}

	if err := newTestHandler(model, store, &fakeSender{}).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "удали трату 1"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	for call := 1; call <= 2; call++ {
		if result := model.toolResult(t, call); !strings.Contains(result, domain.ErrNotFound.Error()) {
			t.Fatalf("result %d = %s", call, result)
		}
	}
	if got, err := store.Get(context.Background(), 66, foreign.ID); err != nil || got.Amount != 50000 {
		t.Fatalf("foreign = %+v, %v", got, err)
	}
}

func TestInvalidToolArgumentsGoBackToModel(t *testing.T) {
	tests := []struct {
		name string
		call chat.Message
		want string
	}{
		{name: "negative amount", call: toolCall("c", toolAdd, `{"amount":-5,"category":"еда"}`), want: domain.ErrInvalidAmount.Error()},
		{name: "unknown category", call: toolCall("c", toolAdd, `{"amount":5,"category":"кофе"}`), want: domain.ErrInvalidCategory.Error()},
		{name: "future date", call: toolCall("c", toolAdd, `{"amount":5,"category":"еда","spent_at":"2026-10-31"}`), want: domain.ErrFutureDate.Error()},
		{name: "bad date", call: toolCall("c", toolList, `{"from":"вчера"}`), want: domain.ErrInvalidDate.Error()},
		{name: "reversed range", call: toolCall("c", toolList, `{"from":"2026-10-30","to":"2026-10-01"}`), want: errBadRange.Error()},
		{name: "empty update", call: toolCall("c", toolUpdate, `{"id":1}`), want: errNoChanges.Error()},
		{name: "future update", call: toolCall("c", toolUpdate, `{"id":1,"spent_at":"2026-11-01"}`), want: domain.ErrFutureDate.Error()},
		{name: "broken json", call: toolCall("c", toolAdd, `{"amount":`), want: errBadArgs.Error()},
		{name: "unknown tool", call: toolCall("c", "drop_table", `{}`), want: errUnknownTool.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &memoryStore{}
			model := &scriptedModel{steps: []chat.Message{tt.call, reply("Не получилось.")}}
			if err := newTestHandler(model, store, &fakeSender{}).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "текст"}); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if result := model.toolResult(t, 1); !strings.Contains(result, tt.want) {
				t.Fatalf("result = %s, want %q", result, tt.want)
			}
			if len(store.items) != 0 {
				t.Fatalf("items = %+v", store.items)
			}
		})
	}
}

func TestStoreFailureIsHiddenFromModel(t *testing.T) {
	store := &memoryStore{err: errors.New("connection refused to 10.0.0.5")}
	model := &scriptedModel{steps: []chat.Message{
		toolCall("c1", toolAdd, `{"amount":350,"category":"еда"}`),
		reply("Не получилось записать."),
	}}
	if err := newTestHandler(model, store, &fakeSender{}).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "кофе 350"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	result := model.toolResult(t, 1)
	if strings.Contains(result, "10.0.0.5") || !strings.Contains(result, errStore.Error()) {
		t.Fatalf("result = %s", result)
	}
}

func TestListPassesFiltersAndTotals(t *testing.T) {
	store := &memoryStore{}
	_, _ = store.Add(context.Background(), seed(55, 35000, domain.CategoryFood, "кофе"))
	_, _ = store.Add(context.Background(), seed(55, 15050, domain.CategoryFood, "булочка"))
	model := &scriptedModel{steps: []chat.Message{
		toolCall("c1", toolList, `{"from":"2026-10-01","to":"2026-10-30","category":"еда","amount":"350","limit":50}`),
		reply("За месяц на еду 500,50 ₽."),
	}}
	if err := newTestHandler(model, store, &fakeSender{}).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "сколько на еду в этом месяце"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	query := store.lastQuery
	if query.UserID != 55 || query.Category != domain.CategoryFood || query.Amount != 35000 || query.Limit != maxListLimit ||
		!query.From.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || !query.To.Equal(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("query = %+v", query)
	}
	if result := model.toolResult(t, 1); !strings.Contains(result, `"total":"350.00"`) || !strings.Contains(result, `"count":1`) {
		t.Fatalf("result = %s", result)
	}
}

func TestUpdateChangesOnlyGivenFields(t *testing.T) {
	store := &memoryStore{}
	_, _ = store.Add(context.Background(), seed(55, 35000, domain.CategoryFood, "кофе"))
	model := &scriptedModel{steps: []chat.Message{
		toolCall("c1", toolUpdate, `{"id":1,"amount":450}`),
		reply("Изменил: кофе, было 350 ₽, стало 450 ₽."),
	}}
	if err := newTestHandler(model, store, &fakeSender{}).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "последнее было не 350, а 450"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := store.items[0]
	if got.Amount != 45000 || got.Comment != "кофе" || got.Category != domain.CategoryFood {
		t.Fatalf("got = %+v", got)
	}
	if result := model.toolResult(t, 1); !strings.Contains(result, `"before"`) || !strings.Contains(result, `"amount":"450.00"`) {
		t.Fatalf("result = %s", result)
	}
}

func TestHistoryCarriesToolResultsToNextMessage(t *testing.T) {
	store := &memoryStore{}
	model := &scriptedModel{steps: []chat.Message{
		toolCall("c1", toolList, `{}`),
		reply("Список."),
		reply("Ок."),
		reply("Привет."),
	}}
	handler := newTestHandler(model, store, &fakeSender{})
	ctx := context.Background()
	if err := handler.Handle(ctx, Command{UserID: 55, ChatID: 77, Text: "покажи траты"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := handler.Handle(ctx, Command{UserID: 55, ChatID: 77, Text: "удали вторую"}); err != nil {
		t.Fatalf("second: %v", err)
	}
	if err := handler.Handle(ctx, Command{UserID: 66, ChatID: 88, Text: "привет"}); err != nil {
		t.Fatalf("third: %v", err)
	}

	second := model.calls[2]
	roles := make([]chat.Role, 0, len(second))
	for _, msg := range second {
		roles = append(roles, msg.Role)
	}
	want := []chat.Role{chat.RoleUser, chat.RoleAssistant, chat.RoleTool, chat.RoleAssistant, chat.RoleUser}
	if len(roles) != len(want) {
		t.Fatalf("roles = %v", roles)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("roles = %v", roles)
		}
	}
	if second[len(second)-1].Text != "удали вторую" {
		t.Fatalf("last = %+v", second[len(second)-1])
	}
	if other := model.calls[3]; len(other) != 1 || other[0].Text != "привет" {
		t.Fatalf("another user saw history: %+v", other)
	}
}

func TestHistoryKeepsRecentExchanges(t *testing.T) {
	model := &scriptedModel{}
	handler := newTestHandler(model, &memoryStore{}, &fakeSender{})
	for i := range maxExchanges + 2 {
		model.steps = append(model.steps, reply("ок"))
		if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: strings.Repeat("я", i+1)}); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}
	if got := len(handler.recent(55)); got != maxExchanges*2 {
		t.Fatalf("history messages = %d", got)
	}
}

func TestModelFailureSendsApology(t *testing.T) {
	sender := &fakeSender{}
	err := newTestHandler(&scriptedModel{err: errors.New("llm down")}, &memoryStore{}, sender).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "кофе 350"})
	if err == nil || !strings.Contains(sender.last(), "Попробуй ещё раз") {
		t.Fatalf("err = %v, sent = %q", err, sender.last())
	}
}

func TestEndlessToolLoopStops(t *testing.T) {
	model := &scriptedModel{}
	handler := newTestHandler(model, &memoryStore{}, &fakeSender{})
	err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "покажи"})
	if !errors.Is(err, errTooManySteps) || len(model.calls) != maxSteps {
		t.Fatalf("err = %v, calls = %d", err, len(model.calls))
	}
	if len(handler.recent(55)) != 0 {
		t.Fatal("history must not keep a failed exchange")
	}
}

func TestCommandsSkipModel(t *testing.T) {
	model := &scriptedModel{}
	sender := &fakeSender{}
	if err := newTestHandler(model, &memoryStore{}, sender).Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "/help"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(model.calls) != 0 || !strings.Contains(sender.last(), "Команды не нужны") {
		t.Fatalf("calls = %d, sent = %q", len(model.calls), sender.last())
	}
}

func TestHandleRequiresUser(t *testing.T) {
	err := newTestHandler(&scriptedModel{}, &memoryStore{}, &fakeSender{}).Handle(context.Background(), Command{ChatID: 77, Text: "кофе 350"})
	if !errors.Is(err, domain.ErrUserRequired) {
		t.Fatalf("err = %v", err)
	}
}

package expense

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

type fakeModel struct {
	called  bool
	intent  domain.Intent
	err     error
	history []domain.Turn
}

func (f *fakeModel) Recognize(_ context.Context, _ string, _ time.Time, history []domain.Turn) (domain.Intent, error) {
	f.called = true
	f.history = append([]domain.Turn(nil), history...)
	return f.intent, f.err
}

type fakeStore struct {
	item          domain.Expense
	items         []domain.Expense
	called        bool
	err           error
	total         int64
	count         int
	listQuery     domain.Query
	totalQuery    domain.Query
	findQuery     domain.Query
	findErr       error
	updatedUser   int64
	updatedID     int64
	updatedAmount int64
	deletedUser   int64
	deletedID     int64
}

func (f *fakeStore) Add(_ context.Context, item domain.Expense) (domain.Expense, error) {
	f.called = true
	f.item = item
	if f.err != nil {
		return domain.Expense{}, f.err
	}
	item.ID = 7
	return item, nil
}

func (f *fakeStore) List(_ context.Context, query domain.Query) ([]domain.Expense, error) {
	f.listQuery = query
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func (f *fakeStore) Total(_ context.Context, query domain.Query) (int64, int, error) {
	f.totalQuery = query
	if f.err != nil {
		return 0, 0, f.err
	}
	return f.total, f.count, nil
}

func (f *fakeStore) Get(_ context.Context, userID, id int64) (domain.Expense, error) {
	for _, item := range f.items {
		if item.ID == id && item.UserID == userID {
			return item, nil
		}
	}
	if f.item.ID == id && f.item.UserID == userID {
		return f.item, nil
	}
	return domain.Expense{}, domain.ErrNotFound
}

func (f *fakeStore) FindLatest(_ context.Context, query domain.Query) (domain.Expense, error) {
	f.findQuery = query
	if f.findErr != nil {
		return domain.Expense{}, f.findErr
	}
	if f.item.ID == 0 {
		return domain.Expense{}, domain.ErrNotFound
	}
	return f.item, nil
}

func (f *fakeStore) Update(_ context.Context, userID, id int64, patch domain.Patch) (domain.Expense, error) {
	f.updatedUser = userID
	f.updatedID = id
	f.updatedAmount = patch.Amount
	if f.err != nil {
		return domain.Expense{}, f.err
	}
	if f.item.UserID != userID {
		return domain.Expense{}, domain.ErrNotFound
	}
	saved := f.item
	if patch.Amount > 0 {
		saved.Amount = patch.Amount
	}
	if patch.Category != "" {
		saved.Category = patch.Category
	}
	if !patch.SpentAt.IsZero() {
		saved.SpentAt = patch.SpentAt
	}
	if patch.SetComment {
		saved.Comment = patch.Comment
	}
	return saved, nil
}

func (f *fakeStore) Delete(_ context.Context, userID, id int64) (domain.Expense, error) {
	f.deletedUser = userID
	f.deletedID = id
	if f.err != nil {
		return domain.Expense{}, f.err
	}
	if f.item.UserID != userID {
		return domain.Expense{}, domain.ErrNotFound
	}
	return f.item, nil
}

type fakeSender struct {
	chatID int64
	text   string
	err    error
}

func (f *fakeSender) SendText(_ context.Context, chatID int64, text string) error {
	f.chatID = chatID
	f.text = text
	return f.err
}

func TestHandleSavesOnlySenderExpense(t *testing.T) {
	today := time.Date(2026, 10, 30, 21, 0, 0, 0, time.FixedZone("+05", 5*60*60))
	model := &fakeModel{intent: domain.Intent{
		Action:   domain.ActionAdd,
		Amount:   35000,
		Category: domain.CategoryFood,
		SpentAt:  time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
		Comment:  "кофе",
	}}
	store := &fakeStore{}
	sender := &fakeSender{}
	handler := newTestHandler(model, store, sender, today)

	err := handler.Handle(context.Background(), Command{
		UserID: 55,
		ChatID: 77,
		Text:   "кофе 350",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !model.called {
		t.Fatal("model was not called")
	}
	if !store.called || store.item.UserID != 55 || store.item.Amount != 35000 || store.item.Category != domain.CategoryFood {
		t.Fatalf("stored = %+v", store.item)
	}
	if sender.chatID != 77 {
		t.Fatalf("reply chat = %d", sender.chatID)
	}
	const want = "Записал: кофе — 350 ₽, еда, 30.10.2026."
	if sender.text != want {
		t.Fatalf("reply = %q, want %q", sender.text, want)
	}
}

func TestHandleUsesModelForUnstructuredText(t *testing.T) {
	today := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	model := &fakeModel{intent: domain.Intent{
		Action:   domain.ActionAdd,
		Amount:   200000,
		Category: domain.CategoryShopping,
		SpentAt:  time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC),
		Comment:  "подарок",
	}}
	store := &fakeStore{}
	sender := &fakeSender{}
	handler := newTestHandler(model, store, sender, today)

	err := handler.Handle(context.Background(), Command{
		UserID: 4,
		ChatID: 4,
		Text:   "купил подарок маме вчера за две тысячи",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !model.called {
		t.Fatal("model was not called")
	}
	if store.item.UserID != 4 || store.item.Comment != "подарок" || store.item.Amount != 200000 {
		t.Fatalf("stored = %+v", store.item)
	}
	if sender.text != "Записал: подарок — 2000 ₽, покупки, 29.10.2026." {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleDoesNotSaveUnknownText(t *testing.T) {
	model := &fakeModel{err: domain.ErrNotExpense}
	store := &fakeStore{}
	sender := &fakeSender{}
	handler := newTestHandler(model, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	err := handler.Handle(context.Background(), Command{UserID: 1, ChatID: 2, Text: "привет"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.called {
		t.Fatal("expense was saved")
	}
	if sender.chatID != 2 || sender.text == "" {
		t.Fatalf("reply = %#v", sender)
	}
}

func TestHandleRejectsFutureDate(t *testing.T) {
	today := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	model := &fakeModel{intent: domain.Intent{
		Action:   domain.ActionAdd,
		Amount:   10000,
		Category: domain.CategoryTransport,
		SpentAt:  time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
		Comment:  "такси",
	}}
	store := &fakeStore{}
	sender := &fakeSender{}
	handler := newTestHandler(model, store, sender, today)

	if err := handler.Handle(context.Background(), Command{UserID: 1, ChatID: 1, Text: "такси завтра"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.called {
		t.Fatal("future expense was saved")
	}
	if sender.text != "Дата траты не может быть позже сегодняшнего дня." {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleReportsStoreError(t *testing.T) {
	today := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{err: errors.New("db down")}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action:   domain.ActionAdd,
		Amount:   35000,
		Category: domain.CategoryFood,
		SpentAt:  today,
		Comment:  "кофе",
	}}, store, sender, today)

	err := handler.Handle(context.Background(), Command{UserID: 9, ChatID: 9, Text: "кофе 350"})
	if !errors.Is(err, store.err) {
		t.Fatalf("error = %v", err)
	}
	if sender.text != "Не получилось записать трату. Попробуй ещё раз." {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleListsFoodForMonth(t *testing.T) {
	today := time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC)
	store := &fakeStore{
		total: 35000,
		count: 1,
		items: []domain.Expense{{
			ID:       1,
			UserID:   55,
			Amount:   35000,
			Category: domain.CategoryFood,
			SpentAt:  time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
			Comment:  "кофе",
		}},
	}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action:   domain.ActionList,
		Category: domain.CategoryFood,
		Period:   domain.PeriodMonth,
	}}, store, sender, today)

	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "сколько потратил на еду в этом месяце"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.totalQuery.UserID != 55 || store.listQuery.UserID != 55 {
		t.Fatalf("query user total=%d list=%d", store.totalQuery.UserID, store.listQuery.UserID)
	}
	monthStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if !store.listQuery.From.Equal(monthStart) || !store.listQuery.To.Equal(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("bounds = %s..%s", store.listQuery.From, store.listQuery.To)
	}
	if store.listQuery.Category != domain.CategoryFood {
		t.Fatalf("category = %s", store.listQuery.Category)
	}
	const want = "В этом месяце, еда: 350 ₽\n• 30.10.2026 — кофе, 350 ₽, еда"
	if sender.chatID != 77 || sender.text != want {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleListsWeek(t *testing.T) {
	today := time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC)
	store := &fakeStore{total: 120000, count: 12, items: []domain.Expense{{
		ID: 2, UserID: 3, Amount: 120000, Category: domain.CategoryTransport,
		SpentAt: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC), Comment: "такси",
	}}}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action: domain.ActionList,
		Period: domain.PeriodWeek,
	}}, store, sender, today)

	if err := handler.Handle(context.Background(), Command{UserID: 3, ChatID: 3, Text: "покажи траты за неделю"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !store.listQuery.From.Equal(time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("week from = %s", store.listQuery.From)
	}
	if !strings.Contains(sender.text, "За неделю: 1200 ₽") || !strings.Contains(sender.text, "И ещё 11.") {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleListsEmptyPeriod(t *testing.T) {
	store := &fakeStore{}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action: domain.ActionList,
		Period: domain.PeriodMonth,
	}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 1, ChatID: 1, Text: "покажи траты"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if sender.text != "За этот период трат нет." {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleUpdatesLatestAmount(t *testing.T) {
	spent := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{item: domain.Expense{
		ID: 9, UserID: 55, Amount: 35000, Category: domain.CategoryFood, SpentAt: spent, Comment: "кофе",
	}}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action:    domain.ActionUpdate,
		Amount:    35000,
		NewAmount: 45000,
	}}, store, sender, spent)

	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "последнее было не 350, а 450"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.findQuery.UserID != 55 || store.findQuery.Amount != 35000 {
		t.Fatalf("find = %+v", store.findQuery)
	}
	if store.updatedUser != 55 || store.updatedID != 9 || store.updatedAmount != 45000 {
		t.Fatalf("update user=%d id=%d amount=%d", store.updatedUser, store.updatedID, store.updatedAmount)
	}
	if sender.text != "Изменил: кофе — было 350 ₽, стало 450 ₽." {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleUpdateRequiresNewAmount(t *testing.T) {
	store := &fakeStore{}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{Action: domain.ActionUpdate}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 1, ChatID: 1, Text: "измени последнее"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.updatedID != 0 {
		t.Fatal("expense was updated")
	}
	if !strings.Contains(sender.text, "450") {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleDeletesMatchedExpense(t *testing.T) {
	store := &fakeStore{item: domain.Expense{
		ID: 4, UserID: 55, Amount: 120000, Category: domain.CategoryTransport,
		SpentAt: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC), Comment: "такси",
	}}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action: domain.ActionDelete,
		Match:  "такси",
	}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "удали такси"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.findQuery.UserID != 55 || store.findQuery.Match != "такси" {
		t.Fatalf("find = %+v", store.findQuery)
	}
	if store.deletedUser != 55 || store.deletedID != 4 {
		t.Fatalf("delete user=%d id=%d", store.deletedUser, store.deletedID)
	}
	if !strings.HasPrefix(sender.text, "Удалил: ") || !strings.Contains(sender.text, "такси") {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleDeleteAsksWhenTargetIsMissing(t *testing.T) {
	store := &fakeStore{}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{Action: domain.ActionDelete}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 1, ChatID: 1, Text: "удали"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.deletedID != 0 {
		t.Fatal("expense was deleted")
	}
	if !strings.Contains(sender.text, "удали такси") {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleMissingExpense(t *testing.T) {
	store := &fakeStore{findErr: domain.ErrNotFound}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action: domain.ActionDelete,
		Match:  "такси",
	}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 1, ChatID: 1, Text: "удали такси"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if sender.text != "Не нашёл такую трату среди своих записей." {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleListsCustomDay(t *testing.T) {
	day := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{total: 120000, count: 1, items: []domain.Expense{{
		ID: 2, UserID: 3, Amount: 120000, Category: domain.CategoryTransport,
		SpentAt: day, Comment: "такси",
	}}}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action: domain.ActionList,
		From:   day,
		To:     day,
	}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 3, ChatID: 3, Text: "что по тратам за вчера"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !store.listQuery.From.Equal(day) || !store.listQuery.To.Equal(day) {
		t.Fatalf("bounds = %s..%s", store.listQuery.From, store.listQuery.To)
	}
	if !strings.HasPrefix(sender.text, "За 29.10.2026:") {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleDeletesSecondShownExpense(t *testing.T) {
	day := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		total: 105000,
		count: 2,
		items: []domain.Expense{
			{ID: 11, UserID: 55, Amount: 60000, Category: domain.CategoryFood, SpentAt: day, Comment: "Кофе"},
			{ID: 22, UserID: 55, Amount: 45000, Category: domain.CategoryFood, SpentAt: day, Comment: "Кофе"},
		},
		item: domain.Expense{ID: 22, UserID: 55, Amount: 45000, Category: domain.CategoryFood, SpentAt: day, Comment: "Кофе"},
	}
	model := &fakeModel{intent: domain.Intent{Action: domain.ActionList}}
	sender := &fakeSender{}
	handler := newTestHandler(model, store, sender, day)

	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "Покажи мои записанные затраты"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	model.intent = domain.Intent{Action: domain.ActionDelete, Match: "траут"}
	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "Удали вторую траут"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if store.deletedUser != 55 || store.deletedID != 22 {
		t.Fatalf("deleted user=%d id=%d reply=%q", store.deletedUser, store.deletedID, sender.text)
	}
}

func TestHandleDeletesCopiedListLine(t *testing.T) {
	day := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{item: domain.Expense{
		ID: 22, UserID: 55, Amount: 45000, Category: domain.CategoryFood, SpentAt: day, Comment: "Кофе",
	}}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{Action: domain.ActionDelete, Match: "трату"}}, store, sender, day)

	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "Удали трату 30.10.2026 — Кофе, 450 ₽, еда"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if store.findQuery.Amount != 45000 || store.findQuery.Match != "Кофе" || store.findQuery.Category != domain.CategoryFood {
		t.Fatalf("find = %+v", store.findQuery)
	}
	if !store.findQuery.From.Equal(day) || store.deletedID != 22 {
		t.Fatalf("from = %s deleted = %d", store.findQuery.From, store.deletedID)
	}
}

func TestHandleDeletesLatest(t *testing.T) {
	store := &fakeStore{item: domain.Expense{
		ID: 4, UserID: 55, Amount: 35000, Category: domain.CategoryFood,
		SpentAt: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), Comment: "кофе",
	}}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action: domain.ActionDelete,
		Latest: true,
	}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "убери последнюю"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.findQuery.UserID != 55 || store.findQuery.Match != "" || store.deletedID != 4 {
		t.Fatalf("find = %+v, deleted = %d", store.findQuery, store.deletedID)
	}
}

func TestHandlePassesRecentDialogue(t *testing.T) {
	today := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	model := &fakeModel{intent: domain.Intent{
		Action: domain.ActionAdd, Amount: 35000, Category: domain.CategoryFood, SpentAt: today, Comment: "кофе",
	}}
	handler := newTestHandler(model, &fakeStore{}, &fakeSender{}, today)
	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "кофе 350"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	model.intent = domain.Intent{Action: domain.ActionList, Period: domain.PeriodWeek}
	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "а за неделю?"}); err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(model.history) != 2 || model.history[0].Text != "кофе 350" || model.history[0].Role != domain.RoleUser {
		t.Fatalf("history = %+v", model.history)
	}
	if model.history[1].Role != domain.RoleAssistant || !strings.Contains(model.history[1].Text, "Записал") {
		t.Fatalf("reply history = %+v", model.history[1])
	}
}

func TestFallbackUsesSimpleParseWhenModelIsDown(t *testing.T) {
	today := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	model := &fakeModel{err: errors.New("groq unavailable")}
	recognizer := WithFallback(model)

	intent, err := recognizer.Recognize(context.Background(), "кофе 350", today, nil)
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if !model.called || intent.Action != domain.ActionAdd || intent.Amount != 35000 || intent.Category != domain.CategoryFood || intent.Comment != "кофе" {
		t.Fatalf("intent = %+v, called = %v", intent, model.called)
	}
}

func TestFallbackKeepsModelRejection(t *testing.T) {
	model := &fakeModel{err: domain.ErrNotExpense}
	_, err := WithFallback(model).Recognize(context.Background(), "кофе 350", time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), nil)
	if !errors.Is(err, domain.ErrNotExpense) {
		t.Fatalf("error = %v", err)
	}
}

func newTestHandler(model Model, store Store, sender Sender, today time.Time) *Handler {
	handler := NewHandler(model, store, sender, time.UTC)
	handler.now = func() time.Time { return today }
	return handler
}

type namedOnlyStore struct {
	fakeStore
}

func (s *namedOnlyStore) FindLatest(_ context.Context, query domain.Query) (domain.Expense, error) {
	s.findQuery = query
	if query.Match != "" {
		return domain.Expense{}, domain.ErrNotFound
	}
	return s.item, nil
}

func TestHandleDeleteDoesNotPickOtherExpense(t *testing.T) {
	store := &namedOnlyStore{fakeStore{item: domain.Expense{
		ID: 9, UserID: 55, Amount: 50000, Category: domain.CategoryTransport,
		SpentAt: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), Comment: "метро",
	}}}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action:   domain.ActionDelete,
		Match:    "такси",
		Amount:   50000,
		Category: domain.CategoryTransport,
	}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "удали такси 500"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.deletedID != 0 {
		t.Fatalf("deleted unrelated expense %d", store.deletedID)
	}
	if sender.text != "Не нашёл такую трату среди своих записей." {
		t.Fatalf("reply = %q", sender.text)
	}
}

func TestHandleUpdateFindsByOldAmount(t *testing.T) {
	store := &namedOnlyStore{fakeStore{item: domain.Expense{
		ID: 9, UserID: 55, Amount: 35000, Category: domain.CategoryFood,
		SpentAt: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), Comment: "кофе",
	}}}
	sender := &fakeSender{}
	handler := newTestHandler(&fakeModel{intent: domain.Intent{
		Action:    domain.ActionUpdate,
		Match:     "латте",
		Amount:    35000,
		NewAmount: 45000,
	}}, store, sender, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

	if err := handler.Handle(context.Background(), Command{UserID: 55, ChatID: 77, Text: "латте было не 350, а 450"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.updatedID != 9 || store.updatedAmount != 45000 {
		t.Fatalf("update id=%d amount=%d", store.updatedID, store.updatedAmount)
	}
}

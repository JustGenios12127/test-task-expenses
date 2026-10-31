package family

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

func TestDisplayNamePrefersTelegramNick(t *testing.T) {
	if got := DisplayName("  @ivan ", "Иван"); got != "@ivan" {
		t.Fatalf("name = %q", got)
	}
	if got := DisplayName("", " Анна "); got != "Анна" {
		t.Fatalf("name = %q", got)
	}
	if got := DisplayName("", " "); got != "без ника" {
		t.Fatalf("name = %q", got)
	}
}

func TestListUsesStoredNickWithoutLookup(t *testing.T) {
	store := &fakeStore{rows: []domain.FamilyExpense{{
		ID: 1, UserID: 10, Username: "mark", FirstName: "Марк", HasProfile: true,
		Amount: 35000, Category: domain.CategoryFood,
	}}}
	profiles := &fakeProfiles{username: "other", firstName: "Другой"}
	page, err := NewService(store, profiles, nil).List(context.Background(), Query{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if profiles.calls != 0 || len(page.Items) != 1 || page.Items[0].Name != "@mark" {
		t.Fatalf("items = %+v, lookups = %d", page.Items, profiles.calls)
	}
}

func TestListFillsMissingNickFromTelegram(t *testing.T) {
	store := &fakeStore{rows: []domain.FamilyExpense{
		{ID: 2, UserID: 20, Amount: 120000, Category: domain.CategoryTransport},
		{ID: 1, UserID: 20, Amount: 35000, Category: domain.CategoryFood},
	}}
	profiles := &fakeProfiles{username: "anna", firstName: "Анна"}
	page, err := NewService(store, profiles, nil).List(context.Background(), Query{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if profiles.calls != 1 || len(store.saved) != 1 || store.saved[0].id != 20 || store.saved[0].username != "anna" {
		t.Fatalf("lookups = %d, saved = %+v", profiles.calls, store.saved)
	}
	if page.Items[0].Name != "@anna" || page.Items[1].Name != "@anna" {
		t.Fatalf("names = %q, %q", page.Items[0].Name, page.Items[1].Name)
	}
}

func TestListKeepsWorkingWhenTelegramFails(t *testing.T) {
	store := &fakeStore{rows: []domain.FamilyExpense{{ID: 3, UserID: 30, Amount: 100}}}
	profiles := &fakeProfiles{err: errors.New("telegram down")}
	page, err := NewService(store, profiles, nil).List(context.Background(), Query{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(store.saved) != 0 || page.Items[0].Name != "без ника" {
		t.Fatalf("saved = %+v, name = %q", store.saved, page.Items[0].Name)
	}
}

func TestCreateUsesKnownMember(t *testing.T) {
	store := &fakeStore{members: []domain.FamilyMember{{ID: 10, Username: "ivan", FirstName: "Иван"}}}
	svc := NewService(store, &fakeProfiles{}, nil)
	svc.now = func() time.Time { return time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC) }

	item, err := svc.Create(context.Background(), Input{
		UserID: 10, Amount: "350,5", Category: "еда", SpentAt: "2026-10-30", Comment: " кофе ",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Name != "@ivan" || item.Amount != 35050 || item.Comment != "кофе" || store.added.UserID != 10 {
		t.Fatalf("item = %+v, added = %+v", item, store.added)
	}
}

func TestCreateRejectsUnknownMemberAndFutureDate(t *testing.T) {
	store := &fakeStore{members: []domain.FamilyMember{{ID: 10, Username: "ivan"}}}
	svc := NewService(store, &fakeProfiles{}, nil)
	svc.now = func() time.Time { return time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC) }

	_, err := svc.Create(context.Background(), Input{
		UserID: 99, Amount: "10", Category: "еда", SpentAt: "2026-10-30",
	})
	if !errors.Is(err, ErrUnknownMember) {
		t.Fatalf("unknown member error = %v", err)
	}

	_, err = svc.Create(context.Background(), Input{
		UserID: 10, Amount: "10", Category: "еда", SpentAt: "2026-10-31",
	})
	if !errors.Is(err, domain.ErrFutureDate) {
		t.Fatalf("future date error = %v", err)
	}
}

func TestListPassesFiltersAndTotal(t *testing.T) {
	store := &fakeStore{rows: []domain.FamilyExpense{
		{ID: 1, UserID: 10, HasProfile: true, Amount: 35000},
		{ID: 2, UserID: 10, HasProfile: true, Amount: 15050},
	}}
	page, err := NewService(store, &fakeProfiles{}, nil).List(context.Background(), Query{
		UserID: 10, Category: "жилье", From: "2026-10-01", To: "2026-10-31",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := domain.FamilyFilter{
		UserID:   10,
		Category: domain.CategoryHousing,
		From:     time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		To:       time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
	}
	if store.filter != want {
		t.Fatalf("filter = %+v", store.filter)
	}
	if page.Total != 50050 || page.Count != 2 {
		t.Fatalf("page = %+v", page)
	}
}

func TestListRejectsBadFilters(t *testing.T) {
	svc := NewService(&fakeStore{}, &fakeProfiles{}, nil)
	for _, query := range []Query{
		{Category: "хак"},
		{From: "2026-13-01"},
		{From: "2026-10-31", To: "2026-10-01"},
	} {
		if _, err := svc.List(context.Background(), query); !errors.Is(err, ErrInvalidFilter) {
			t.Fatalf("query %+v error = %v", query, err)
		}
	}
}

func TestUpdatePassesVersion(t *testing.T) {
	store := &fakeStore{members: []domain.FamilyMember{{ID: 10, Username: "ivan"}}}
	svc := NewService(store, &fakeProfiles{}, nil)
	svc.now = func() time.Time { return time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC) }
	input := Input{UserID: 10, Amount: "10", Category: "еда", SpentAt: "2026-10-30", Version: "2026-10-30T10:00:00.123456Z"}

	if _, err := svc.Update(context.Background(), 7, input); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !store.version.Equal(time.Date(2026, 10, 30, 10, 0, 0, 123456000, time.UTC)) {
		t.Fatalf("version = %s", store.version)
	}

	input.Version = "вчера"
	if _, err := svc.Update(context.Background(), 7, input); !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("bad version error = %v", err)
	}
}

type savedUser struct {
	id        int64
	username  string
	firstName string
}

type fakeStore struct {
	rows     []domain.FamilyExpense
	members  []domain.FamilyMember
	saved    []savedUser
	added    domain.Expense
	replaced domain.Expense
	version  time.Time
	deleted  int64
	filter   domain.FamilyFilter
}

func (f *fakeStore) ListFamily(_ context.Context, filter domain.FamilyFilter) ([]domain.FamilyExpense, error) {
	f.filter = filter
	return f.rows, nil
}

func (f *fakeStore) FamilyTotal(context.Context, domain.FamilyFilter) (int64, int, error) {
	var total int64
	for _, row := range f.rows {
		total += row.Amount
	}
	return total, len(f.rows), nil
}

func (f *fakeStore) SaveUser(_ context.Context, id int64, username, firstName string) error {
	f.saved = append(f.saved, savedUser{id: id, username: username, firstName: firstName})
	return nil
}

func (f *fakeStore) ListMembers(context.Context) ([]domain.FamilyMember, error) {
	return f.members, nil
}

func (f *fakeStore) FindMember(_ context.Context, id int64) (domain.FamilyMember, error) {
	for _, member := range f.members {
		if member.ID == id {
			return member, nil
		}
	}
	return domain.FamilyMember{}, domain.ErrNotFound
}

func (f *fakeStore) Add(_ context.Context, item domain.Expense) (domain.Expense, error) {
	item.ID = 1
	f.added = item
	return item, nil
}

func (f *fakeStore) Replace(_ context.Context, id int64, item domain.Expense, version time.Time) (domain.Expense, error) {
	if id != 7 {
		return domain.Expense{}, domain.ErrNotFound
	}
	item.ID = id
	f.replaced = item
	f.version = version
	return item, nil
}

func (f *fakeStore) DeleteByID(_ context.Context, id int64) error {
	if id != 7 {
		return domain.ErrNotFound
	}
	f.deleted = id
	return nil
}

type fakeProfiles struct {
	username  string
	firstName string
	err       error
	calls     int
}

func (f *fakeProfiles) Profile(context.Context, int64) (string, string, error) {
	f.calls++
	if f.err != nil {
		return "", "", f.err
	}
	return f.username, f.firstName, nil
}

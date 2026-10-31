package family

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

var (
	ErrUnknownMember  = errors.New("unknown member")
	ErrInvalidFilter  = errors.New("invalid filter")
	ErrInvalidVersion = errors.New("invalid version")
)

type Store interface {
	ListFamily(ctx context.Context, filter domain.FamilyFilter) ([]domain.FamilyExpense, error)
	FamilyTotal(ctx context.Context, filter domain.FamilyFilter) (int64, int, error)
	ListMembers(ctx context.Context) ([]domain.FamilyMember, error)
	FindMember(ctx context.Context, id int64) (domain.FamilyMember, error)
	SaveUser(ctx context.Context, id int64, username, firstName string) error
	Add(ctx context.Context, item domain.Expense) (domain.Expense, error)
	Replace(ctx context.Context, id int64, item domain.Expense, version time.Time) (domain.Expense, error)
	DeleteByID(ctx context.Context, id int64) error
}

type Query struct {
	UserID   int64
	Category string
	From     string
	To       string
}

type Page struct {
	Items []Expense
	Total int64
	Count int
}

type Profiles interface {
	Profile(ctx context.Context, userID int64) (username, firstName string, err error)
}

type Expense struct {
	ID        int64
	UserID    int64
	Name      string
	Amount    int64
	Category  domain.Category
	SpentAt   time.Time
	Comment   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Member struct {
	ID   int64
	Name string
}

type Input struct {
	UserID   int64
	Amount   string
	Category string
	SpentAt  string
	Comment  string
	Version  string
}

type Service struct {
	store    Store
	profiles Profiles
	now      func() time.Time
}

func NewService(store Store, profiles Profiles, loc *time.Location) *Service {
	if loc == nil {
		loc = time.UTC
	}
	return &Service{
		store:    store,
		profiles: profiles,
		now: func() time.Time {
			return time.Now().In(loc)
		},
	}
}

func (s *Service) List(ctx context.Context, query Query) (Page, error) {
	filter, err := parseQuery(query)
	if err != nil {
		return Page{}, err
	}
	total, count, err := s.store.FamilyTotal(ctx, filter)
	if err != nil {
		return Page{}, err
	}
	rows, err := s.store.ListFamily(ctx, filter)
	if err != nil {
		return Page{}, err
	}

	resolved := map[int64][2]string{}
	for _, row := range rows {
		if row.HasProfile {
			continue
		}
		if _, seen := resolved[row.UserID]; seen {
			continue
		}
		username, firstName, err := s.lookup(ctx, row.UserID)
		if err != nil {
			slog.Warn("telegram profile", "err", err, "user_id", row.UserID)
			resolved[row.UserID] = [2]string{}
			continue
		}
		if err := s.store.SaveUser(ctx, row.UserID, username, firstName); err != nil {
			slog.Warn("save telegram profile", "err", err, "user_id", row.UserID)
		}
		resolved[row.UserID] = [2]string{username, firstName}
	}

	items := make([]Expense, 0, len(rows))
	for _, row := range rows {
		username := row.Username
		firstName := row.FirstName
		if !row.HasProfile {
			pair := resolved[row.UserID]
			username = pair[0]
			firstName = pair[1]
		}
		items = append(items, Expense{
			ID:        row.ID,
			UserID:    row.UserID,
			Name:      DisplayName(username, firstName),
			Amount:    row.Amount,
			Category:  row.Category,
			SpentAt:   row.SpentAt,
			Comment:   row.Comment,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return Page{Items: items, Total: total, Count: count}, nil
}

func parseQuery(query Query) (domain.FamilyFilter, error) {
	filter := domain.FamilyFilter{UserID: query.UserID}
	if query.UserID < 0 {
		return domain.FamilyFilter{}, ErrInvalidFilter
	}
	if raw := strings.TrimSpace(query.Category); raw != "" {
		category, err := domain.ParseCategory(raw)
		if err != nil {
			return domain.FamilyFilter{}, ErrInvalidFilter
		}
		filter.Category = category
	}
	for _, field := range []struct {
		raw  string
		dest *time.Time
	}{{query.From, &filter.From}, {query.To, &filter.To}} {
		raw := strings.TrimSpace(field.raw)
		if raw == "" {
			continue
		}
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return domain.FamilyFilter{}, ErrInvalidFilter
		}
		*field.dest = parsed
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && filter.To.Before(filter.From) {
		return domain.FamilyFilter{}, ErrInvalidFilter
	}
	return filter, nil
}

func (s *Service) Members(ctx context.Context) ([]Member, error) {
	rows, err := s.store.ListMembers(ctx)
	if err != nil {
		return nil, err
	}
	members := make([]Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, Member{
			ID:   row.ID,
			Name: DisplayName(row.Username, row.FirstName),
		})
	}
	return members, nil
}

func (s *Service) Create(ctx context.Context, input Input) (Expense, error) {
	member, item, err := s.prepare(ctx, input)
	if err != nil {
		return Expense{}, err
	}
	saved, err := s.store.Add(ctx, item)
	if err != nil {
		return Expense{}, err
	}
	return present(saved, member), nil
}

func (s *Service) Update(ctx context.Context, id int64, input Input) (Expense, error) {
	if id <= 0 {
		return Expense{}, domain.ErrNotFound
	}
	var version time.Time
	if raw := strings.TrimSpace(input.Version); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return Expense{}, ErrInvalidVersion
		}
		version = parsed
	}
	member, item, err := s.prepare(ctx, input)
	if err != nil {
		return Expense{}, err
	}
	saved, err := s.store.Replace(ctx, id, item, version)
	if err != nil {
		return Expense{}, err
	}
	return present(saved, member), nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return domain.ErrNotFound
	}
	return s.store.DeleteByID(ctx, id)
}

func (s *Service) prepare(ctx context.Context, input Input) (domain.FamilyMember, domain.Expense, error) {
	if input.UserID <= 0 {
		return domain.FamilyMember{}, domain.Expense{}, ErrUnknownMember
	}
	member, err := s.store.FindMember(ctx, input.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.FamilyMember{}, domain.Expense{}, ErrUnknownMember
	}
	if err != nil {
		return domain.FamilyMember{}, domain.Expense{}, err
	}

	amount, err := domain.ParseAmount(input.Amount)
	if err != nil {
		return domain.FamilyMember{}, domain.Expense{}, err
	}
	category, err := domain.ParseCategory(input.Category)
	if err != nil {
		return domain.FamilyMember{}, domain.Expense{}, err
	}
	spent, err := time.Parse("2006-01-02", strings.TrimSpace(input.SpentAt))
	if err != nil {
		return domain.FamilyMember{}, domain.Expense{}, domain.ErrInvalidDate
	}
	item, err := domain.New(member.ID, domain.Draft{
		Amount:   amount,
		Category: category,
		SpentAt:  spent,
		Comment:  input.Comment,
	}, s.now())
	if err != nil {
		return domain.FamilyMember{}, domain.Expense{}, err
	}
	return member, item, nil
}

func present(item domain.Expense, member domain.FamilyMember) Expense {
	return Expense{
		ID:        item.ID,
		UserID:    item.UserID,
		Name:      DisplayName(member.Username, member.FirstName),
		Amount:    item.Amount,
		Category:  item.Category,
		SpentAt:   item.SpentAt,
		Comment:   item.Comment,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

func (s *Service) lookup(ctx context.Context, userID int64) (string, string, error) {
	if s.profiles == nil {
		return "", "", fmt.Errorf("telegram profiles are not configured")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	return s.profiles.Profile(lookupCtx, userID)
}

func DisplayName(username, firstName string) string {
	nick := strings.TrimPrefix(strings.TrimSpace(username), "@")
	if nick != "" {
		return "@" + nick
	}
	name := strings.TrimSpace(firstName)
	if name != "" {
		return name
	}
	return "без ника"
}

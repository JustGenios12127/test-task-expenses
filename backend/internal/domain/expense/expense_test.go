package expense

import (
	"errors"
	"testing"
	"time"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		raw     string
		want    int64
		wantErr bool
	}{
		{raw: "350", want: 35000},
		{raw: "350.5", want: 35050},
		{raw: "1 250,50", want: 125050},
		{raw: "0", wantErr: true},
		{raw: "-10", wantErr: true},
		{raw: "10.999", wantErr: true},
	}

	for _, tt := range tests {
		got, err := ParseAmount(tt.raw)
		if tt.wantErr {
			if !errors.Is(err, ErrInvalidAmount) {
				t.Fatalf("ParseAmount(%q) error = %v", tt.raw, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseAmount(%q): %v", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("ParseAmount(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestParseSimple(t *testing.T) {
	today := time.Date(2026, 10, 30, 22, 0, 0, 0, time.FixedZone("+05", 5*60*60))

	coffee, err := ParseSimple("кофе 350", today)
	if err != nil {
		t.Fatalf("coffee: %v", err)
	}
	if coffee.Amount != 35000 || coffee.Category != CategoryFood || coffee.Comment != "кофе" {
		t.Fatalf("coffee = %+v", coffee)
	}
	if !coffee.SpentAt.Equal(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("coffee date = %s", coffee.SpentAt)
	}

	taxi, err := ParseSimple("такси 1200 вчера", today)
	if err != nil {
		t.Fatalf("taxi: %v", err)
	}
	if taxi.Amount != 120000 || taxi.Category != CategoryTransport || taxi.Comment != "такси" {
		t.Fatalf("taxi = %+v", taxi)
	}
	if !taxi.SpentAt.Equal(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("taxi date = %s", taxi.SpentAt)
	}

	metro, err := ParseSimple("вчера метро 80", today)
	if err != nil {
		t.Fatalf("metro: %v", err)
	}
	if metro.Category != CategoryTransport || !metro.SpentAt.Equal(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("metro = %+v", metro)
	}

	products, err := ParseSimple("продукты 1 250,50", today)
	if err != nil {
		t.Fatalf("products: %v", err)
	}
	if products.Amount != 125050 || products.Category != CategoryFood {
		t.Fatalf("products = %+v", products)
	}

	cinema, err := ParseSimple("позавчера кино 900", today)
	if err != nil {
		t.Fatalf("cinema: %v", err)
	}
	if cinema.Category != CategoryEntertainment || !cinema.SpentAt.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("cinema = %+v", cinema)
	}

	if _, err := ParseSimple("привет", today); !errors.Is(err, ErrNotExpense) {
		t.Fatalf("greeting error = %v", err)
	}
	if _, err := ParseSimple("завтра такси 500", today); !errors.Is(err, ErrNotExpense) {
		t.Fatalf("tomorrow error = %v", err)
	}
	if _, err := ParseSimple("кофе 2 по 350", today); !errors.Is(err, ErrNotExpense) {
		t.Fatalf("two amounts error = %v", err)
	}
}

func TestNewRejectsForeignAndFutureData(t *testing.T) {
	today := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	draft := Draft{
		Amount:   35000,
		Category: CategoryFood,
		SpentAt:  time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
		Comment:  "кофе",
	}
	if _, err := New(0, draft, today); !errors.Is(err, ErrUserRequired) {
		t.Fatalf("user error = %v", err)
	}
	if _, err := New(42, draft, today); !errors.Is(err, ErrFutureDate) {
		t.Fatalf("future error = %v", err)
	}

	draft.SpentAt = time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	item, err := New(42, draft, today)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if item.UserID != 42 || item.NumericAmount() != "350.00" || FormatRubles(item.Amount) != "350 ₽" {
		t.Fatalf("item = %+v", item)
	}
}

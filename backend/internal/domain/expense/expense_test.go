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

func TestParseCategory(t *testing.T) {
	for raw, want := range map[string]Category{"Еда": CategoryFood, " жилье ": CategoryHousing, "жильё": CategoryHousing} {
		got, err := ParseCategory(raw)
		if err != nil || got != want {
			t.Fatalf("ParseCategory(%q) = %q, %v", raw, got, err)
		}
	}
	if _, err := ParseCategory("кофе"); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("unknown category error = %v", err)
	}
}

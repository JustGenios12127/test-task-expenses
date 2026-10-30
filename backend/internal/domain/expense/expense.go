package expense

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Category string

const (
	CategoryFood          Category = "еда"
	CategoryTransport     Category = "транспорт"
	CategoryHousing       Category = "жильё"
	CategoryEntertainment Category = "развлечения"
	CategoryHealth        Category = "здоровье"
	CategoryShopping      Category = "покупки"
	CategoryOther         Category = "другое"
)

const maxAmount int64 = 9_999_999_999_99
const maxCommentLen = 200

var (
	ErrNotExpense      = errors.New("not an expense")
	ErrInvalidAmount   = errors.New("invalid amount")
	ErrInvalidCategory = errors.New("invalid category")
	ErrInvalidDate     = errors.New("invalid date")
	ErrFutureDate      = errors.New("future date")
	ErrUserRequired    = errors.New("user id is required")
	ErrNotFound        = errors.New("expense not found")
)

type Draft struct {
	Amount   int64
	Category Category
	SpentAt  time.Time
	Comment  string
}

type Expense struct {
	ID        int64
	UserID    int64
	Amount    int64
	Category  Category
	SpentAt   time.Time
	Comment   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func Categories() []Category {
	return []Category{
		CategoryFood,
		CategoryTransport,
		CategoryHousing,
		CategoryEntertainment,
		CategoryHealth,
		CategoryShopping,
		CategoryOther,
	}
}

func (c Category) Valid() bool {
	for _, category := range Categories() {
		if c == category {
			return true
		}
	}
	return false
}

func CheckAmount(kopecks int64) error {
	if kopecks <= 0 || kopecks > maxAmount {
		return ErrInvalidAmount
	}
	return nil
}

func New(userID int64, draft Draft, today time.Time) (Expense, error) {
	if userID <= 0 {
		return Expense{}, ErrUserRequired
	}
	if err := CheckAmount(draft.Amount); err != nil {
		return Expense{}, err
	}
	if !draft.Category.Valid() {
		return Expense{}, ErrInvalidCategory
	}
	spent := DateUTC(draft.SpentAt)
	if spent.IsZero() {
		return Expense{}, ErrInvalidDate
	}
	if spent.After(DateUTC(today)) {
		return Expense{}, ErrFutureDate
	}
	return Expense{
		UserID:   userID,
		Amount:   draft.Amount,
		Category: draft.Category,
		SpentAt:  spent,
		Comment:  truncate(strings.TrimSpace(draft.Comment), maxCommentLen),
	}, nil
}

func (e Expense) NumericAmount() string {
	return fmt.Sprintf("%d.%02d", e.Amount/100, e.Amount%100)
}

func FormatRubles(kopecks int64) string {
	rubles := kopecks / 100
	cents := kopecks % 100
	if cents == 0 {
		return strconv.FormatInt(rubles, 10) + " ₽"
	}
	return fmt.Sprintf("%d,%02d ₽", rubles, cents)
}

func ParseAmount(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "\u00a0", "")
	value = strings.ReplaceAll(value, ",", ".")
	if value == "" || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return 0, ErrInvalidAmount
	}

	whole, frac, found := strings.Cut(value, ".")
	if !found {
		frac = "00"
	}
	if whole == "" {
		whole = "0"
	}
	if strings.Count(value, ".") > 1 || len(frac) > 2 || !digits(whole) || !digits(frac) {
		return 0, ErrInvalidAmount
	}
	for len(frac) < 2 {
		frac += "0"
	}

	rubles, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	amount := rubles*100 + cents
	if amount <= 0 || amount > maxAmount {
		return 0, ErrInvalidAmount
	}
	return amount, nil
}

func ParseCategory(raw string) (Category, error) {
	category := Category(strings.TrimSpace(foldYo(raw)))
	for _, item := range Categories() {
		if foldYo(string(item)) == string(category) {
			return item, nil
		}
	}
	return "", ErrInvalidCategory
}

func NormalizeCategory(raw string) Category {
	if category, err := ParseCategory(raw); err == nil {
		return category
	}
	folded := foldYo(strings.TrimSpace(raw))
	if folded == "" {
		return ""
	}
	if category, ok := categoryAliases[folded]; ok {
		return category
	}
	return ""
}

func CategoryFromText(text string) Category {
	folded := foldYo(text)
	for _, item := range categoryKeywords {
		for _, word := range item.words {
			if strings.Contains(folded, word) {
				return item.category
			}
		}
	}
	return CategoryOther
}

func DateUTC(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func digits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func foldYo(value string) string {
	return strings.ReplaceAll(strings.ToLower(value), "ё", "е")
}

type keywordCategory struct {
	category Category
	words    []string
}

var categoryAliases = map[string]Category{
	"продукты":   CategoryFood,
	"продукт":    CategoryFood,
	"питание":    CategoryFood,
	"еду":        CategoryFood,
	"проезд":     CategoryTransport,
	"дорога":     CategoryTransport,
	"бензин":     CategoryTransport,
	"коммуналка": CategoryHousing,
	"аренда":     CategoryHousing,
	"квартира":   CategoryHousing,
	"лекарства":  CategoryHealth,
	"аптека":     CategoryHealth,
	"медицина":   CategoryHealth,
	"досуг":      CategoryEntertainment,
	"одежда":     CategoryShopping,
	"вещи":       CategoryShopping,
	"прочее":     CategoryOther,
	"разное":     CategoryOther,
}

var categoryKeywords = []keywordCategory{
	{category: CategoryFood, words: []string{"кофе", "кафе", "обед", "ужин", "завтрак", "продукт", "еда", "чай", "ресторан", "пирож", "булоч", "столов"}},
	{category: CategoryTransport, words: []string{"такси", "метро", "автобус", "бензин", "транспорт", "каршер", "электрич", "маршрутка", "самокат"}},
	{category: CategoryHousing, words: []string{"аренд", "квартир", "жкх", "коммунал", "ипотек", "жиль"}},
	{category: CategoryEntertainment, words: []string{"кино", "театр", "игр", "концерт", "развлеч"}},
	{category: CategoryHealth, words: []string{"аптек", "врач", "лекар", "стоматолог", "анализ", "здоров"}},
	{category: CategoryShopping, words: []string{"одежд", "обув", "техник", "покупк"}},
}

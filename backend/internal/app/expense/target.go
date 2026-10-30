package expense

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	domain "test-task-expenses/backend/internal/domain/expense"
)

var (
	errNoShown        = errString("shown list is empty")
	errOutsideList    = errString("shown index is outside the list")
	listedLinePattern = regexp.MustCompile(`(?i)(\d{2})\.(\d{2})\.(\d{4})\s*[—–\-]\s*([^,\n]+?)\s*,\s*(\d+(?:[ \x{00a0}]\d{3})*(?:[.,]\d{1,2})?)\s*(?:₽|р\.?)?(?:\s*,\s*([а-яё]+))?`)
	numericOrdinal    = regexp.MustCompile(`(?i)(?:номер\s*|#\s*)(\d{1,2})\b|\b(\d{1,2})\s*-\s*[яюий]\b`)
)

type errString string

func (e errString) Error() string { return string(e) }

func clarifyTarget(text string, intent domain.Intent) domain.Intent {
	if n := ordinalIndex(text); n > 0 {
		intent.Index = n
	}
	if line, ok := parseListedLine(text); ok {
		intent.Amount = line.Amount
		intent.SpentAt = line.SpentAt
		intent.From = time.Time{}
		intent.To = time.Time{}
		intent.Period = ""
		intent.Category = line.Category
		intent.Match = line.Comment
		intent.Latest = false
		return intent
	}
	if intent.Amount == 0 {
		if amount := loneAmount(text); amount > 0 {
			intent.Amount = amount
		}
	}
	cleaned := cleanMatch(intent.Match)
	if cleaned == "" {
		cleaned = purchaseWords(text)
	}
	intent.Match = cleaned
	return intent
}

func ordinalIndex(text string) int {
	for _, field := range strings.Fields(foldYo(text)) {
		word := trimToken(field)
		if n := ordinalWord(word); n > 0 {
			return n
		}
	}
	match := numericOrdinal.FindStringSubmatch(text)
	if match == nil {
		return 0
	}
	raw := match[1]
	if raw == "" {
		raw = match[2]
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 10 {
		return 0
	}
	return n
}

func ordinalWord(word string) int {
	switch {
	case strings.HasPrefix(word, "перв"):
		return 1
	case strings.HasPrefix(word, "втор"):
		return 2
	case strings.HasPrefix(word, "трет"):
		return 3
	case strings.HasPrefix(word, "четвер"):
		return 4
	case strings.HasPrefix(word, "пят"):
		return 5
	case strings.HasPrefix(word, "шест"):
		return 6
	case strings.HasPrefix(word, "седь"):
		return 7
	case strings.HasPrefix(word, "вось"):
		return 8
	case strings.HasPrefix(word, "девят"):
		return 9
	case strings.HasPrefix(word, "десят"):
		return 10
	default:
		return 0
	}
}

type listedLine struct {
	SpentAt  time.Time
	Comment  string
	Amount   int64
	Category domain.Category
}

func parseListedLine(text string) (listedLine, bool) {
	match := listedLinePattern.FindStringSubmatch(text)
	if match == nil {
		return listedLine{}, false
	}
	day, errDay := strconv.Atoi(match[1])
	month, errMonth := strconv.Atoi(match[2])
	year, errYear := strconv.Atoi(match[3])
	amount, errAmount := domain.ParseAmount(match[5])
	if errDay != nil || errMonth != nil || errYear != nil || errAmount != nil {
		return listedLine{}, false
	}
	spent := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if spent.Day() != day || spent.Month() != time.Month(month) {
		return listedLine{}, false
	}
	return listedLine{
		SpentAt:  spent,
		Comment:  strings.TrimSpace(match[4]),
		Amount:   amount,
		Category: domain.NormalizeCategory(match[6]),
	}, true
}

func loneAmount(text string) int64 {
	var found int64
	count := 0
	for _, field := range strings.Fields(text) {
		word := trimToken(field)
		if strings.Count(word, ".") >= 2 {
			continue
		}
		amount, err := domain.ParseAmount(word)
		if err != nil {
			continue
		}
		found = amount
		count++
	}
	if count != 1 {
		return 0
	}
	return found
}

func cleanMatch(match string) string {
	return purchaseWords(match)
}

func purchaseWords(text string) string {
	kept := make([]string, 0)
	for _, field := range strings.Fields(text) {
		word := trimToken(field)
		folded := foldYo(word)
		if word == "" || skipWord(folded) {
			continue
		}
		if _, err := domain.ParseAmount(word); err == nil {
			continue
		}
		if _, err := domain.ParseCategory(word); err == nil {
			continue
		}
		kept = append(kept, word)
	}
	return strings.Join(kept, " ")
}

func skipWord(word string) bool {
	if ordinalWord(word) > 0 || strings.HasPrefix(word, "послед") {
		return true
	}
	switch word {
	case "удали", "удалить", "убери", "убрать", "сотри", "стереть", "отмени", "отменить",
		"измени", "изменить", "исправь", "поменяй", "поставь", "поставить", "вместо", "было", "стало",
		"трату", "трата", "траты", "траут", "затрату", "затраты", "затрат",
		"мою", "моя", "мой", "мои", "эту", "этот", "это", "строку", "строка", "запись", "записи",
		"номер", "за", "на", "по", "из", "и", "а", "не":
		return true
	default:
		return false
	}
}

func trimToken(value string) string {
	return strings.TrimFunc(value, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSymbol(r) || r == '₽'
	})
}

func foldYo(value string) string {
	return strings.ReplaceAll(strings.ToLower(value), "ё", "е")
}

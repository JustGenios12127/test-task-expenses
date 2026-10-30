package expense

import (
	"regexp"
	"strings"
	"time"
)

var amountPattern = regexp.MustCompile(`(?i)(?:^|\s)(\d{1,3}(?:[ \x{00a0}]\d{3})+|\d+)([.,]\d{1,2})?(?:\s*(?:₽|руб(?:лей|ля|ль|\.)?|р\.?))?(?:$|\s)`)

func ParseSimple(text string, today time.Time) (Draft, error) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 || strings.HasPrefix(fields[0], "/") {
		return Draft{}, ErrNotExpense
	}

	offset, rest, ok := stripDate(fields)
	if !ok {
		return Draft{}, ErrNotExpense
	}

	amountRaw, comment, ok := splitAmount(strings.Join(rest, " "))
	if !ok {
		return Draft{}, ErrNotExpense
	}
	amount, err := ParseAmount(amountRaw)
	if err != nil {
		return Draft{}, ErrNotExpense
	}

	return Draft{
		Amount:   amount,
		Category: CategoryFromText(comment),
		SpentAt:  addDays(today, offset),
		Comment:  strings.Trim(comment, " .,!"),
	}, nil
}

func stripDate(fields []string) (int, []string, bool) {
	offset := 0
	seen := 0
	rest := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.Trim(foldYo(field), ".,!?;:") {
		case "завтра", "послезавтра":
			return 0, nil, false
		case "позавчера":
			offset = -2
			seen++
		case "вчера":
			offset = -1
			seen++
		case "сегодня":
			seen++
		default:
			rest = append(rest, field)
		}
	}
	if seen > 1 {
		return 0, nil, false
	}
	return offset, rest, true
}

func splitAmount(text string) (string, string, bool) {
	matches := amountPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) != 1 {
		return "", "", false
	}
	match := matches[0]
	numberStart, numberEnd := match[2], match[3]
	fracStart, fracEnd := match[4], match[5]
	amount := text[numberStart:numberEnd]
	if fracStart >= 0 {
		amount += text[fracStart:fracEnd]
	}
	comment := strings.TrimSpace(text[:match[0]] + " " + text[match[1]:])
	return amount, strings.Join(strings.Fields(comment), " "), true
}

func addDays(today time.Time, days int) time.Time {
	year, month, day := today.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
}

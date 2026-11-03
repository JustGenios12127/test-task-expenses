package expense

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

//go:embed prompts/system.tmpl
var systemTemplateText string

var systemTemplate = template.Must(template.New("system").Option("missingkey=error").Parse(systemTemplateText))

var weekdays = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}

type promptData struct {
	Today      string
	Weekday    string
	Categories string
}

func systemPrompt(today time.Time) (string, error) {
	names := make([]string, 0, len(domain.Categories()))
	for _, category := range domain.Categories() {
		names = append(names, string(category))
	}
	var out strings.Builder
	err := systemTemplate.Execute(&out, promptData{
		Today:      today.Format(dateLayout),
		Weekday:    weekdays[today.Weekday()],
		Categories: strings.Join(names, ", "),
	})
	if err != nil {
		return "", fmt.Errorf("render system prompt: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

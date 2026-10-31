package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	domain "test-task-expenses/backend/internal/domain/expense"
)

type Parser struct {
	apiKey     string
	model      string
	url        string
	httpClient *http.Client
}

func NewParser(apiKey, model, url string) *Parser {
	return &Parser{
		apiKey: apiKey,
		model:  model,
		url:    url,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

type chatRequest struct {
	Model          string         `json:"model"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens"`
	ResponseFormat map[string]any `json:"response_format"`
	Messages       []chatMessage  `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type modelIntent struct {
	Action      string    `json:"action"`
	Error       string    `json:"error"`
	Amount      rawAmount `json:"amount"`
	NewAmount   rawAmount `json:"new_amount"`
	Category    string    `json:"category"`
	NewCategory string    `json:"new_category"`
	SpentAt     string    `json:"spent_at"`
	NewSpentAt  string    `json:"new_spent_at"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Period      string    `json:"period"`
	Comment     string    `json:"comment"`
	NewComment  string    `json:"new_comment"`
	SetComment  rawFlag   `json:"set_comment"`
	Match       string    `json:"match"`
	Index       rawAmount `json:"index"`
	Latest      rawFlag   `json:"latest"`
}

type rawAmount struct {
	value string
}

func (a *rawAmount) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		a.value = ""
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		a.value = value
		return nil
	}
	a.value = string(data)
	return nil
}

type rawFlag struct {
	value bool
}

func (f *rawFlag) UnmarshalJSON(data []byte) error {
	switch fold(strings.Trim(string(bytes.TrimSpace(data)), `"`)) {
	case "true", "1", "yes", "да":
		f.value = true
	default:
		f.value = false
	}
	return nil
}

func (p *Parser) Recognize(ctx context.Context, text string, today time.Time, history []domain.Turn) (domain.Intent, error) {
	body := chatRequest{
		Model:       p.model,
		Temperature: 0,
		MaxTokens:   1200,
		ResponseFormat: map[string]any{
			"type": "json_object",
		},
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt(today)},
			{Role: "user", Content: userContent(text, history)},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return domain.Intent{}, fmt.Errorf("marshal groq request: %w", err)
	}

	resp, raw, err := p.post(ctx, payload)
	if err != nil {
		return domain.Intent{}, err
	}
	if isRateLimit(resp, raw) {
		timer := time.NewTimer(15 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return domain.Intent{}, ctx.Err()
		case <-timer.C:
		}
		resp, raw, err = p.post(ctx, payload)
		if err != nil {
			return domain.Intent{}, err
		}
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return domain.Intent{}, fmt.Errorf("decode groq response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || parsed.Error != nil {
		message := ""
		if parsed.Error != nil {
			message = parsed.Error.Message
		}
		if message == "" {
			message = resp.Status
		}
		return domain.Intent{}, fmt.Errorf("groq response: %s", message)
	}
	if len(parsed.Choices) == 0 {
		return domain.Intent{}, fmt.Errorf("groq response: empty choices")
	}
	intent, err := intentFromModel(parsed.Choices[0].Message.Content, today)
	if err != nil {
		return domain.Intent{}, err
	}
	return fillAddComment(intent, text, today), nil
}

func (p *Parser) post(ctx context.Context, payload []byte) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("build groq request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("groq request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("read groq response: %w", err)
	}
	return resp, raw, nil
}

func isRateLimit(resp *http.Response, raw []byte) bool {
	if resp == nil || resp.StatusCode == http.StatusOK {
		return false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return strings.Contains(strings.ToLower(string(raw)), "rate limit")
}

func userContent(text string, history []domain.Turn) string {
	text = limitRunes(strings.TrimSpace(text), 500)
	if len(history) == 0 {
		return text
	}
	if len(history) > 6 {
		history = history[len(history)-6:]
	}
	var builder strings.Builder
	builder.WriteString("Диалог:\n")
	for _, turn := range history {
		who := "Пользователь"
		if turn.Role == domain.RoleAssistant {
			who = "Бот"
		}
		fmt.Fprintf(&builder, "%s: %s\n", who, limitRunes(strings.TrimSpace(turn.Text), 300))
	}
	fmt.Fprintf(&builder, "Новое сообщение: %s", text)
	return builder.String()
}

func intentFromModel(content string, today time.Time) (domain.Intent, error) {
	var parsed modelIntent
	if err := json.Unmarshal([]byte(extractJSON(content)), &parsed); err != nil {
		return domain.Intent{}, domain.ErrNotExpense
	}
	if parsed.Error != "" || fold(parsed.Action) == "unknown" {
		return domain.Intent{}, domain.ErrNotExpense
	}

	action := parseAction(parsed.Action)
	amount, err := optionalAmount(parsed.Amount.value)
	if err != nil {
		return domain.Intent{}, domain.ErrNotExpense
	}
	newAmount, err := optionalAmount(parsed.NewAmount.value)
	if err != nil {
		return domain.Intent{}, domain.ErrNotExpense
	}
	if action == "" && amount > 0 {
		action = domain.ActionAdd
	}
	if action == "" {
		return domain.Intent{}, domain.ErrNotExpense
	}

	spentAt, err := parseDate(parsed.SpentAt, today)
	if err != nil {
		return domain.Intent{}, domain.ErrNotExpense
	}
	newSpentAt, err := parseDate(parsed.NewSpentAt, today)
	if err != nil {
		return domain.Intent{}, domain.ErrNotExpense
	}
	from, err := parseDate(parsed.From, today)
	if err != nil {
		return domain.Intent{}, domain.ErrNotExpense
	}
	to, err := parseDate(parsed.To, today)
	if err != nil {
		return domain.Intent{}, domain.ErrNotExpense
	}

	intent := domain.Intent{
		Action:      action,
		Amount:      amount,
		NewAmount:   newAmount,
		Category:    optionalCategory(parsed.Category),
		NewCategory: optionalCategory(parsed.NewCategory),
		SpentAt:     spentAt,
		NewSpentAt:  newSpentAt,
		From:        from,
		To:          to,
		Period:      parsePeriod(parsed.Period),
		Comment:     strings.TrimSpace(parsed.Comment),
		Match:       strings.TrimSpace(parsed.Match),
		Latest:      parsed.Latest.value,
		Index:       optionalIndex(parsed.Index.value),
	}
	if comment := strings.TrimSpace(parsed.NewComment); comment != "" {
		intent.NewComment = comment
		intent.SetComment = true
	} else if parsed.SetComment.value {
		intent.SetComment = true
	}
	if action == domain.ActionAdd {
		if intent.Amount <= 0 {
			return domain.Intent{}, domain.ErrNotExpense
		}
		if intent.SpentAt.IsZero() {
			intent.SpentAt = domain.DateUTC(today)
		}
		if intent.Category == "" {
			intent.Category = domain.CategoryFromText(intent.Comment)
		}
	}
	return intent, nil
}

func fillAddComment(intent domain.Intent, text string, today time.Time) domain.Intent {
	if intent.Action != domain.ActionAdd || strings.TrimSpace(intent.Comment) != "" {
		return intent
	}
	simple, err := domain.ParseSimple(text, today)
	if err != nil || simple.Amount != intent.Amount || strings.TrimSpace(simple.Comment) == "" {
		return intent
	}
	intent.Comment = simple.Comment
	return intent
}

func parseAction(raw string) domain.Action {
	switch fold(raw) {
	case "add", "create", "добавить", "запиши", "запись":
		return domain.ActionAdd
	case "list", "show", "view", "посмотреть", "покажи", "список", "сколько":
		return domain.ActionList
	case "update", "change", "edit", "изменить", "исправь", "поменяй":
		return domain.ActionUpdate
	case "delete", "remove", "удалить", "убери", "сотри", "отмени":
		return domain.ActionDelete
	default:
		return ""
	}
}

func parsePeriod(raw string) domain.Period {
	switch fold(raw) {
	case "day", "день", "сегодня", "today":
		return domain.PeriodDay
	case "week", "неделя", "неделю", "неделе", "недели":
		return domain.PeriodWeek
	case "month", "месяц", "месяце", "месяца":
		return domain.PeriodMonth
	default:
		return ""
	}
}

func parseDate(raw string, today time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, today.Location())
	if err != nil {
		return time.Time{}, err
	}
	return domain.DateUTC(parsed), nil
}

func optionalAmount(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	return domain.ParseAmount(raw)
}

func optionalIndex(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, ".,") {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 20 {
		return 0
	}
	return n
}

func optionalCategory(raw string) domain.Category {
	return domain.NormalizeCategory(raw)
}

func fold(raw string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), "ё", "е")
}

func systemPrompt(today time.Time) string {
	names := make([]string, 0, len(domain.Categories()))
	for _, category := range domain.Categories() {
		names = append(names, string(category))
	}
	yesterday := today.AddDate(0, 0, -1).Format("2006-01-02")
	return fmt.Sprintf(`Ты разбираешь обычную речь про личные траты. Человек пишет как удобно: вопросом, с суммой словами, с другим порядком слов, с опечатками. Не жди готовую фразу.
Сегодня %s. Все даты считай от этого дня.
Категории строго: %s.
Верни только JSON:
{"action":"add|list|update|delete|unknown","amount":"","new_amount":"","category":"","new_category":"","spent_at":"","new_spent_at":"","from":"","to":"","period":"","comment":"","new_comment":"","match":"","index":0,"latest":false,"set_comment":false}
Пустые поля — пустые строки. Не выдумывай сумму, дату, категорию и название. Не указывай владельца.
add — человек сообщает новую трату. amount числом в рублях. spent_at — дата траты, если не названа то сегодня. category из списка, если понятна. comment — что куплено, коротко и своими словами человека.
list — хочет посмотреть или посчитать. Если назван срок, заполни from и to включительно. period=day только для сегодня, week для последних 7 дней включая сегодня, month для текущего месяца с 1-го числа по сегодня. Иначе period пустой. category — если названа категория. match — если названа покупка.
update — исправляет уже записанную трату. new_amount — новая сумма. amount — старая, если названа. new_category, new_spent_at и new_comment заполняй только когда просит изменить именно это. set_comment=true, если просит стереть комментарий. match — какую трату искать. latest=true, если говорит «последнюю», «её» или «эту» без нового названия.
delete — убрать одну трату. match — только название, например «кофе», не вся фраза. amount — сумма, если названа. index — номер строки последнего списка с 1, «вторую» значит 2. latest=true для «последнюю» без номера.
unknown — приветствие, благодарность и всё, что не про траты.
Суммы: «триста пятьдесят»=350, «полторы тысячи»=1500, «2к»=2000, «1 250,50»=1250.50.
Даты: вчера=%s, позавчера — минус 2 дня, «3 дня назад» — минус 3. «за неделю», «на этой неделе» и «за последние семь дней» — сегодня минус 6 дней и сегодня, period=week. «в этом месяце» и «с начала месяца» — с 1-го числа по сегодня, period=month. «в прошлом месяце» — весь предыдущий календарный месяц, period пустой. «с понедельника» — с понедельника этой недели по сегодня.
Если перед новым сообщением есть диалог, короткие уточнения вроде «а за еду?», «нет, 450», «удали её» достраивай из него. Новое сообщение важнее истории.
В одном сообщении несколько трат — разбери только первую.
Пример при сегодня %s: «сегодня взял латте за триста пятьдесят» → add, amount 350, category еда, spent_at %s, comment латте.
«заправил машину на 3 тысячи вчера» → add, amount 3000, category транспорт, spent_at %s, comment заправка.
«а сколько я уже спустил на продукты с начала месяца» → list, category еда, period month, from первое число, to сегодня.
«что по тратам за вчера» → list, from %s, to %s.
«кофе на самом деле стоил не 350, а 450» → update, match кофе, amount 350, new_amount 450.
«перенеси такси на вчера» → update, match такси, new_spent_at %s.
«убери последнюю» → delete, latest true.
«привет» → unknown.`, today.Format("2006-01-02"), strings.Join(names, ", "), yesterday, today.Format("2006-01-02"), today.Format("2006-01-02"), yesterday, yesterday, yesterday, yesterday)
}

func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	start := strings.IndexByte(raw, '{')
	end := strings.LastIndexByte(raw, '}')
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}

func limitRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

package expense

import (
	"strings"
	"time"
)

type Action string

const (
	ActionAdd    Action = "add"
	ActionList   Action = "list"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

type Period string

const (
	PeriodDay   Period = "day"
	PeriodWeek  Period = "week"
	PeriodMonth Period = "month"
)

type Intent struct {
	Action      Action
	Amount      int64
	NewAmount   int64
	Category    Category
	NewCategory Category
	SpentAt     time.Time
	NewSpentAt  time.Time
	From        time.Time
	To          time.Time
	Period      Period
	Comment     string
	NewComment  string
	SetComment  bool
	Match       string
	Latest      bool
	Index       int
}

type Turn struct {
	Role string
	Text string
}

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

type Patch struct {
	Amount     int64
	Category   Category
	SpentAt    time.Time
	Comment    string
	SetComment bool
}

func (p Patch) Empty() bool {
	return p.Amount == 0 && p.Category == "" && p.SpentAt.IsZero() && !p.SetComment
}

func (p Patch) Normalized() Patch {
	if !p.SpentAt.IsZero() {
		p.SpentAt = DateUTC(p.SpentAt)
	}
	if p.SetComment {
		p.Comment = truncate(strings.TrimSpace(p.Comment), maxCommentLen)
	}
	return p
}

type Query struct {
	UserID   int64
	Category Category
	From     time.Time
	To       time.Time
	Match    string
	Amount   int64
	Limit    int
}

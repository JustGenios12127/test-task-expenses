package expense

import (
	"strings"
	"time"
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

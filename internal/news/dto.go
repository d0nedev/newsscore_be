package news

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultLimit = 20
	maxLimit     = 50
	maxSlugLen   = 80
)

// ---- public ----

type listFilter struct {
	MatchID, TeamID, PlayerID *uuid.UUID
	Limit                     int
	After                     *cursor
}

type cursor struct {
	PublishedAt time.Time
	ID          uuid.UUID
}

func (c cursor) encode() string {
	raw := strconv.FormatInt(c.PublishedAt.UnixMicro(), 10) + ":" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(v string) (cursor, error) {
	invalid := apperror.Validation("invalid cursor")
	raw, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return cursor{}, invalid
	}
	micros, id, ok := strings.Cut(string(raw), ":")
	if !ok {
		return cursor{}, invalid
	}
	ts, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return cursor{}, invalid
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return cursor{}, invalid
	}
	return cursor{PublishedAt: time.UnixMicro(ts), ID: parsed}, nil
}

func parseListFilter(q url.Values) (listFilter, error) {
	f := listFilter{Limit: defaultLimit}

	for name, dst := range map[string]**uuid.UUID{"matchId": &f.MatchID, "teamId": &f.TeamID, "playerId": &f.PlayerID} {
		if v := q.Get(name); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				return f, apperror.Validation("invalid " + name)
			}
			*dst = &id
		}
	}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			return f, apperror.Validation(fmt.Sprintf("limit must be an integer between 1 and %d", maxLimit))
		}
		f.Limit = n
	}

	if v := q.Get("cursor"); v != "" {
		c, err := decodeCursor(v)
		if err != nil {
			return f, err
		}
		f.After = &c
	}

	return f, nil
}

// NewsItem follows the contract: id is the slug, published is a relative label.
type NewsItem struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Image       string `json:"image,omitempty"`
	Published   string `json:"published"`
	PublishedAt string `json:"publishedAt"`
}

type NewsDetail struct {
	NewsItem
	Body string `json:"body"`
}

type ListNewsResponse struct {
	Data []NewsItem `json:"data"`
	Meta struct {
		NextCursor *string `json:"nextCursor"`
	} `json:"meta"`
}

type DataResponse struct {
	Data any `json:"data"`
}

func toNewsItem(slug, category, title, summary string, image pgtype.Text, published pgtype.Timestamptz, now time.Time) NewsItem {
	return NewsItem{
		ID:          slug,
		Category:    category,
		Title:       title,
		Summary:     summary,
		Image:       image.String,
		Published:   relativeLabel(published.Time, now),
		PublishedAt: published.Time.UTC().Format(time.RFC3339),
	}
}

var wib = time.FixedZone("WIB", 7*60*60)

// relativeLabel renders "baru saja", "5 menit lalu", "2 jam lalu", "3 hari lalu", then a date.
func relativeLabel(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "baru saja"
	case d < time.Hour:
		return fmt.Sprintf("%d menit lalu", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d jam lalu", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d hari lalu", int(d.Hours()/24))
	}
	return t.In(wib).Format("02.01.2006")
}

// ---- admin ----

type Links struct {
	MatchIDs  []string `json:"matchIds"`
	TeamIDs   []string `json:"teamIds"`
	PlayerIDs []string `json:"playerIds"`
}

type WriteRequest struct {
	Slug      string `json:"slug"` // optional on create: derived from the title
	Category  string `json:"category"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Body      string `json:"body"`
	Image     string `json:"image"`
	Published bool   `json:"published"`
	Links     Links  `json:"links"`
}

type link struct{ Match, Team, Player pgtype.UUID }

// validated is a WriteRequest after trimming, slug derivation, and id parsing.
type validated struct {
	WriteRequest
	links []link
}

func (req WriteRequest) validate() (validated, error) {
	req.Title = strings.TrimSpace(req.Title)
	req.Category = strings.TrimSpace(req.Category)
	req.Summary = strings.TrimSpace(req.Summary)
	req.Image = strings.TrimSpace(req.Image)
	req.Slug = strings.TrimSpace(req.Slug)
	if req.Slug == "" {
		req.Slug = Slugify(req.Title)
	}

	switch {
	case req.Title == "" || utf8.RuneCountInString(req.Title) > 200:
		return validated{}, apperror.Validation("title is required, at most 200 characters")
	case req.Category == "" || utf8.RuneCountInString(req.Category) > 40:
		return validated{}, apperror.Validation("category is required, at most 40 characters")
	case utf8.RuneCountInString(req.Summary) > 500:
		return validated{}, apperror.Validation("summary must be at most 500 characters")
	case req.Slug == "" || req.Slug != Slugify(req.Slug):
		return validated{}, apperror.Validation("slug must be lowercase letters, digits, and single dashes")
	case req.Image != "" && !strings.HasPrefix(req.Image, "https://"):
		return validated{}, apperror.Validation("image must be an https:// URL")
	}

	v := validated{WriteRequest: req}
	add := func(ids []string, name string, set func(*link, pgtype.UUID)) error {
		for _, s := range ids {
			id, err := uuid.Parse(s)
			if err != nil {
				return apperror.Validation("invalid id in links." + name)
			}
			var l link
			set(&l, pgtype.UUID{Bytes: id, Valid: true})
			v.links = append(v.links, l)
		}
		return nil
	}
	if err := add(req.Links.MatchIDs, "matchIds", func(l *link, id pgtype.UUID) { l.Match = id }); err != nil {
		return validated{}, err
	}
	if err := add(req.Links.TeamIDs, "teamIds", func(l *link, id pgtype.UUID) { l.Team = id }); err != nil {
		return validated{}, err
	}
	if err := add(req.Links.PlayerIDs, "playerIds", func(l *link, id pgtype.UUID) { l.Player = id }); err != nil {
		return validated{}, err
	}
	if len(v.links) > 50 {
		return validated{}, apperror.Validation("at most 50 links")
	}

	return v, nil
}

// Slugify lowercases, maps anything outside a-z0-9 to single dashes, and caps the length.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if len(out) > maxSlugLen {
		out = strings.TrimSuffix(out[:maxSlugLen], "-")
	}
	return out
}

type AdminNews struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Category    string  `json:"category"`
	Title       string  `json:"title"`
	Summary     string  `json:"summary,omitempty"`
	Body        string  `json:"body,omitempty"`
	Image       string  `json:"image,omitempty"`
	PublishedAt *string `json:"publishedAt"`
	UpdatedAt   string  `json:"updatedAt"`
	Links       *Links  `json:"links,omitempty"`
}

func timePtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format(time.RFC3339)
	return &s
}

func toAdminSummary(r db.ListAllNewsRow) AdminNews {
	return AdminNews{
		ID: r.ID.String(), Slug: r.Slug, Category: r.Category, Title: r.Title,
		PublishedAt: timePtr(r.PublishedAt), UpdatedAt: r.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

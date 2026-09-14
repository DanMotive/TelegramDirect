package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	BotToken    string
	AdminChatID int64
	ChannelID   int64
	AdminIDs    map[int64]bool
	DBPath      string
	ConfigPath  string
	Texts       TextConfig
}

type TextConfig struct {
	Bot        BotTextConfig        `json:"bot"`
	Suggestion SuggestionTextConfig `json:"suggestion"`
	Moderation ModerationTextConfig `json:"moderation"`
	Channel    ChannelTextConfig    `json:"channel"`
	Admin      AdminTextConfig      `json:"admin"`
	Errors     ErrorTextConfig      `json:"errors"`
}

type BotTextConfig struct {
	Name            string `json:"name"`
	WelcomeText     string `json:"welcome_text"`
	HelpText        string `json:"help_text"`
	AnonymousButton string `json:"anonymous_button"`
	NamedButton     string `json:"named_button"`
	CancelButton    string `json:"cancel_button"`
	UnknownCommand  string `json:"unknown_command"`
}

type SuggestionTextConfig struct {
	AskText         string `json:"ask_text"`
	ModeAnonymous   string `json:"mode_anonymous"`
	ModeNamed       string `json:"mode_named"`
	ModeSelected    string `json:"mode_selected"`
	EmptyText       string `json:"empty_text"`
	TooLongText     string `json:"too_long_text"`
	SentText        string `json:"sent_text"`
	NoModeText      string `json:"no_mode_text"`
	CancelText      string `json:"cancel_text"`
	AnonymousAuthor string `json:"anonymous_author"`
	MaxLength       int    `json:"max_length"`
}

type ModerationTextConfig struct {
	Title            string `json:"title"`
	IDLabel          string `json:"id_label"`
	AuthorLabel      string `json:"author_label"`
	TextLabel        string `json:"text_label"`
	AnonymousAuthor  string `json:"anonymous_author"`
	NamedFallback    string `json:"named_fallback"`
	ApproveButton    string `json:"approve_button"`
	RejectButton     string `json:"reject_button"`
	ApprovedLog      string `json:"approved_log"`
	RejectedLog      string `json:"rejected_log"`
	AlreadyProcessed string `json:"already_processed"`
	NotFound         string `json:"not_found"`
	InvalidID        string `json:"invalid_id"`
	NoPermission     string `json:"no_permission"`
	PublishedCallback string `json:"published_callback"`
	RejectedCallback  string `json:"rejected_callback"`
}

type ChannelTextConfig struct {
	Header          string `json:"header"`
	ShowID          bool   `json:"show_id"`
	ShowAuthor      bool   `json:"show_author"`
	AuthorPrefix    string `json:"author_prefix"`
	AnonymousAuthor string `json:"anonymous_author"`
}

type AdminTextConfig struct {
	QueueTitle   string `json:"queue_title"`
	QueueItem    string `json:"queue_item"`
	QueueEmpty   string `json:"queue_empty"`
	HelpText     string `json:"help_text"`
	PublishedLog string `json:"published_log"`
	RejectedLog  string `json:"rejected_log"`
}

type ErrorTextConfig struct {
	Generic       string `json:"generic"`
	NotAllowed    string `json:"not_allowed"`
	Unavailable   string `json:"unavailable"`
	InvalidAction string `json:"invalid_action"`
	InvalidMode   string `json:"invalid_mode"`
}

type BotAPI struct {
	Token  string
	Client *http.Client
}

type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgMessage struct {
	MessageID int     `json:"message_id"`
	From      *tgUser `json:"from"`
	Chat      tgChat  `json:"chat"`
	Text      string  `json:"text"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    tgUser     `json:"from"`
	Message *tgMessage `json:"message"`
	Data    string     `json:"data"`
}

type tgUpdate struct {
	UpdateID      int               `json:"update_id"`
	Message       *tgMessage        `json:"message"`
	CallbackQuery *tgCallbackQuery  `json:"callback_query"`
}

type tgResponse[T any] struct {
	OK          bool            `json:"ok"`
	Result      T               `json:"result"`
	Description string          `json:"description"`
}

type Suggestion struct {
	ID            int64
	UserID        int64
	Username      string
	DisplayName   string
	Text          string
	Anonymous     bool
	Status        string
	CreatedAt     time.Time
	ReviewedBy    int64
	ReviewMessage int
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := initDB(db); err != nil {
		log.Fatal(err)
	}

	api := &BotAPI{
		Token:  cfg.BotToken,
		Client: &http.Client{Timeout: 60 * time.Second},
	}

	ctx := context.Background()

	me, err := api.GetMe(ctx)
	if err != nil {
		log.Fatal("telegram authentication failed: ", err)
	}

	log.Printf("bot started: @%s (%d)", me.Username, me.ID)

	var offset int

	for {
		updates, err := api.GetUpdates(ctx, offset, 50, 50)
		if err != nil {
			log.Printf("polling error: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}

			if err := handleUpdate(ctx, api, db, cfg, upd); err != nil {
				log.Printf("update %d failed: %v", upd.UpdateID, err)
			}
		}
	}
}

func loadConfig() (Config, error) {
	botToken := strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	if botToken == "" {
		return Config{}, errors.New("BOT_TOKEN is required")
	}

	adminChatID, err := strconv.ParseInt(
		strings.TrimSpace(os.Getenv("ADMIN_CHAT_ID")),
		10,
		64,
	)
	if err != nil {
		return Config{}, fmt.Errorf("invalid ADMIN_CHAT_ID: %w", err)
	}

	channelID, err := strconv.ParseInt(
		strings.TrimSpace(os.Getenv("CHANNEL_ID")),
		10,
		64,
	)
	if err != nil {
		return Config{}, fmt.Errorf("invalid CHANNEL_ID: %w", err)
	}

	adminIDs := make(map[int64]bool)

	for _, raw := range strings.Split(os.Getenv("ADMIN_IDS"), ",") {
		raw = strings.TrimSpace(raw)

		if raw == "" {
			continue
		}

		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("invalid admin ID %q: %w", raw, err)
		}

		adminIDs[id] = true
	}

	if len(adminIDs) == 0 {
		return Config{}, errors.New("ADMIN_IDS must contain at least one Telegram user ID")
	}

	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		dbPath = "data.db"
	}

	configPath := strings.TrimSpace(os.Getenv("CONFIG_PATH"))
	if configPath == "" {
		configPath = "config.json"
	}

	texts, err := loadTextConfig(configPath)
	if err != nil {
		return Config{}, err
	}

	return Config{
		BotToken:    botToken,
		AdminChatID: adminChatID,
		ChannelID:   channelID,
		AdminIDs:    adminIDs,
		DBPath:      dbPath,
		ConfigPath:  configPath,
		Texts:       texts,
	}, nil
}

func loadTextConfig(path string) (TextConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TextConfig{}, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	var cfg TextConfig

	if err := json.Unmarshal(data, &cfg); err != nil {
		return TextConfig{}, fmt.Errorf("invalid JSON config %s: %w", path, err)
	}

	if cfg.Suggestion.MaxLength <= 0 {
		cfg.Suggestion.MaxLength = 4000
	}

	return cfg, nil
}

func initDB(db *sql.DB) error {
	_, err := db.Exec(`
		PRAGMA journal_mode=WAL;

		CREATE TABLE IF NOT EXISTS suggestions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL,
			anonymous INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			reviewed_by INTEGER NOT NULL DEFAULT 0,
			review_message INTEGER NOT NULL DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_suggestions_status
		ON suggestions(status);

		CREATE TABLE IF NOT EXISTS user_states (
			user_id INTEGER PRIMARY KEY,
			mode INTEGER NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)

	return err
}

func handleUpdate(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	upd tgUpdate,
) error {
	if upd.CallbackQuery != nil {
		return handleCallback(ctx, api, db, cfg, upd.CallbackQuery)
	}

	if upd.Message != nil {
		return handleMessage(ctx, api, db, cfg, upd.Message)
	}

	return nil
}

func handleMessage(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	msg *tgMessage,
) error {
	if msg.From == nil || msg.From.IsBot {
		return nil
	}

	if msg.Chat.ID == cfg.AdminChatID && cfg.AdminIDs[msg.From.ID] {
		return handleAdminMessage(ctx, api, db, cfg, msg)
	}

	if msg.Chat.ID != msg.From.ID {
		return nil
	}

	switch strings.TrimSpace(msg.Text) {
	case "/start", "/suggest":
		return sendChoiceKeyboard(
			ctx,
			api,
			msg.Chat.ID,
			cfg.Texts.Bot.WelcomeText,
			cfg,
		)
	}

	if strings.HasPrefix(msg.Text, "/") {
		return api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Bot.UnknownCommand,
			nil,
		)
	}

	var mode int

	err := db.QueryRow(
		`SELECT mode FROM user_states WHERE user_id=?`,
		msg.From.ID,
	).Scan(&mode)

	if err == sql.ErrNoRows {
		return api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Suggestion.NoModeText,
			nil,
		)
	}

	if err != nil {
		return err
	}

	if err := submitSuggestion(
		ctx,
		api,
		db,
		cfg,
		*msg.From,
		msg.Text,
		mode == 1,
	); err != nil {
		return err
	}

	_, _ = db.Exec(
		`DELETE FROM user_states WHERE user_id=?`,
		msg.From.ID,
	)

	return nil
}

func handleAdminMessage(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	msg *tgMessage,
) error {
	switch strings.TrimSpace(msg.Text) {
	case "/queue":
		return sendQueue(ctx, api, db, cfg)

	case "/help":
		return api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Admin.HelpText,
			nil,
		)

	default:
		return nil
	}
}

func handleCallback(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	cb *tgCallbackQuery,
) error {
	if cb.Message == nil {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Errors.Unavailable,
		)
	}

	parts := strings.SplitN(cb.Data, ":", 2)

	if len(parts) != 2 {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Errors.InvalidAction,
		)
	}

	switch parts[0] {
	case "submit":
		if cb.Message.Chat.ID != cb.From.ID {
			return api.AnswerCallbackQuery(
				ctx,
				cb.ID,
				cfg.Texts.Errors.NotAllowed,
			)
		}

		mode, err := strconv.Atoi(parts[1])

		if err != nil || (mode != 0 && mode != 1) {
			return api.AnswerCallbackQuery(
				ctx,
				cb.ID,
				cfg.Texts.Errors.InvalidMode,
			)
		}

		_, err = db.Exec(`
			INSERT INTO user_states (user_id, mode)
			VALUES (?, ?)
			ON CONFLICT(user_id)
			DO UPDATE SET
				mode=excluded.mode,
				updated_at=CURRENT_TIMESTAMP
		`, cb.From.ID, mode)

		if err != nil {
			return err
		}

		label := cfg.Texts.Suggestion.ModeAnonymous
		if mode == 0 {
			label = cfg.Texts.Suggestion.ModeNamed
		}

		if err := api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			fmt.Sprintf(cfg.Texts.Suggestion.ModeSelected, label),
		); err != nil {
			return err
		}

		return api.SendMessage(
			ctx,
			cb.From.ID,
			fmt.Sprintf(cfg.Texts.Suggestion.AskText, label),
			nil,
		)

	case "approve":
		if cb.Message.Chat.ID != cfg.AdminChatID ||
			!cfg.AdminIDs[cb.From.ID] {
			return api.AnswerCallbackQuery(
				ctx,
				cb.ID,
				cfg.Texts.Moderation.NoPermission,
			)
		}

		return approveSuggestion(
			ctx,
			api,
			db,
			cfg,
			cb,
			parts[1],
		)

	case "reject":
		if cb.Message.Chat.ID != cfg.AdminChatID ||
			!cfg.AdminIDs[cb.From.ID] {
			return api.AnswerCallbackQuery(
				ctx,
				cb.ID,
				cfg.Texts.Moderation.NoPermission,
			)
		}

		return rejectSuggestion(
			ctx,
			api,
			db,
			cfg,
			cb,
			parts[1],
		)

	default:
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Errors.InvalidAction,
		)
	}
}

func sendChoiceKeyboard(
	ctx context.Context,
	api *BotAPI,
	chatID int64,
	text string,
	cfg Config,
) error {
	markup := map[string]any{
		"inline_keyboard": [][]map[string]string{
			{
				{
					"text":          cfg.Texts.Bot.AnonymousButton,
					"callback_data": "submit:1",
				},
				{
					"text":          cfg.Texts.Bot.NamedButton,
					"callback_data": "submit:0",
				},
			},
		},
	}

	return api.SendMessage(
		ctx,
		chatID,
		text,
		markup,
	)
}

func submitSuggestion(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	user tgUser,
	text string,
	anonymous bool,
) error {
	text = strings.TrimSpace(text)

	if text == "" {
		return api.SendMessage(
			ctx,
			user.ID,
			cfg.Texts.Suggestion.EmptyText,
			nil,
		)
	}

	if len([]rune(text)) > cfg.Texts.Suggestion.MaxLength {
		return api.SendMessage(
			ctx,
			user.ID,
			fmt.Sprintf(
				cfg.Texts.Suggestion.TooLongText,
				cfg.Texts.Suggestion.MaxLength,
			),
			nil,
		)
	}

	displayName := strings.TrimSpace(
		strings.Join(
			[]string{
				user.FirstName,
				user.LastName,
			},
			" ",
		),
	)

	_, err := db.Exec(`
		INSERT INTO suggestions
		(user_id, username, display_name, text, anonymous)
		VALUES (?, ?, ?, ?, ?)
	`,
		user.ID,
		user.Username,
		displayName,
		text,
		boolInt(anonymous),
	)

	if err != nil {
		return err
	}

	var (
		s       Suggestion
		anonInt int
	)

	row := db.QueryRow(`
		SELECT
			id,
			user_id,
			username,
			display_name,
			text,
			anonymous,
			status,
			created_at
		FROM suggestions
		ORDER BY id DESC
		LIMIT 1
	`)

	if err := row.Scan(
		&s.ID,
		&s.UserID,
		&s.Username,
		&s.DisplayName,
		&s.Text,
		&anonInt,
		&s.Status,
		&s.CreatedAt,
	); err != nil {
		return err
	}

	s.Anonymous = anonInt != 0

	if _, err := sendModerationMessage(
		ctx,
		api,
		cfg,
		s,
	); err != nil {
		return err
	}

	return api.SendMessage(
		ctx,
		user.ID,
		cfg.Texts.Suggestion.SentText,
		nil,
	)
}

func sendModerationMessage(
	ctx context.Context,
	api *BotAPI,
	cfg Config,
	s Suggestion,
) (int, error) {
	author := formatSuggestionAuthor(
		cfg,
		s,
		cfg.Texts.Moderation.AnonymousAuthor,
		cfg.Texts.Moderation.NamedFallback,
	)

	text := fmt.Sprintf(
		"%s\n\n%s: #%d\n%s: %s\n\n%s:\n%s",
		cfg.Texts.Moderation.Title,
		cfg.Texts.Moderation.IDLabel,
		s.ID,
		cfg.Texts.Moderation.AuthorLabel,
		author,
		cfg.Texts.Moderation.TextLabel,
		s.Text,
	)

	markup := map[string]any{
		"inline_keyboard": [][]map[string]string{
			{
				{
					"text": cfg.Texts.Moderation.ApproveButton,
					"callback_data": fmt.Sprintf(
						"approve:%d",
						s.ID,
					),
				},
				{
					"text": cfg.Texts.Moderation.RejectButton,
					"callback_data": fmt.Sprintf(
						"reject:%d",
						s.ID,
					),
				},
			},
		},
	}

	message, err := api.SendMessageWithResult(
		ctx,
		cfg.AdminChatID,
		text,
		markup,
	)

	if err != nil {
		return 0, err
	}

	return message.MessageID, nil
}

func approveSuggestion(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	cb *tgCallbackQuery,
	rawID string,
) error {
	id, err := strconv.ParseInt(rawID, 10, 64)

	if err != nil {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Moderation.InvalidID,
		)
	}

	s, err := getSuggestion(db, id)

	if err != nil {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Moderation.NotFound,
		)
	}

	if s.Status != "pending" {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Moderation.AlreadyProcessed,
		)
	}

	channelText := buildChannelMessage(cfg, s)

	if err := api.SendMessage(
		ctx,
		cfg.ChannelID,
		channelText,
		nil,
	); err != nil {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Errors.Generic,
		)
	}

	_, err = db.Exec(`
		UPDATE suggestions
		SET
			status='approved',
			reviewed_by=?,
			review_message=?
		WHERE id=?
	`,
		cb.From.ID,
		cb.Message.MessageID,
		id,
	)

	if err != nil {
		return err
	}

	logLine := fmt.Sprintf(
		cfg.Texts.Admin.PublishedLog,
		id,
		userLabel(cb.From),
		cb.From.ID,
	)

	if err := api.SendMessage(
		ctx,
		cfg.AdminChatID,
		logLine,
		nil,
	); err != nil {
		return err
	}

	return api.AnswerCallbackQuery(
		ctx,
		cb.ID,
		cfg.Texts.Moderation.PublishedCallback,
	)
}

func rejectSuggestion(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	cb *tgCallbackQuery,
	rawID string,
) error {
	id, err := strconv.ParseInt(rawID, 10, 64)

	if err != nil {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Moderation.InvalidID,
		)
	}

	s, err := getSuggestion(db, id)

	if err != nil {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Moderation.NotFound,
		)
	}

	if s.Status != "pending" {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Moderation.AlreadyProcessed,
		)
	}

	_, err = db.Exec(`
		UPDATE suggestions
		SET
			status='rejected',
			reviewed_by=?,
			review_message=?
		WHERE id=?
	`,
		cb.From.ID,
		cb.Message.MessageID,
		id,
	)

	if err != nil {
		return err
	}

	logLine := fmt.Sprintf(
		cfg.Texts.Admin.RejectedLog,
		id,
		userLabel(cb.From),
		cb.From.ID,
	)

	if err := api.SendMessage(
		ctx,
		cfg.AdminChatID,
		logLine,
		nil,
	); err != nil {
		return err
	}

	return api.AnswerCallbackQuery(
		ctx,
		cb.ID,
		cfg.Texts.Moderation.RejectedCallback,
	)
}

func buildChannelMessage(
	cfg Config,
	s Suggestion,
) string {
	var b strings.Builder

	if cfg.Texts.Channel.Header != "" {
		b.WriteString(cfg.Texts.Channel.Header)
		b.WriteString("\n\n")
	}

	if cfg.Texts.Channel.ShowID {
		b.WriteString(fmt.Sprintf("#%d\n\n", s.ID))
	}

	b.WriteString(s.Text)

	if cfg.Texts.Channel.ShowAuthor {
		author := formatSuggestionAuthor(
			cfg,
			s,
			cfg.Texts.Channel.AnonymousAuthor,
			"",
		)

		if author != "" {
			b.WriteString("\n\n")
			b.WriteString(cfg.Texts.Channel.AuthorPrefix)
			b.WriteString(" ")
			b.WriteString(author)
		}
	}

	return b.String()
}

func formatSuggestionAuthor(
	cfg Config,
	s Suggestion,
	anonymousLabel string,
	namedFallback string,
) string {
	if s.Anonymous {
		return anonymousLabel
	}

	if s.Username != "" {
		return "@" + s.Username
	}

	if s.DisplayName != "" {
		return s.DisplayName
	}

	if namedFallback != "" {
		return namedFallback
	}

	return "Пользователь"
}

func getSuggestion(
	db *sql.DB,
	id int64,
) (Suggestion, error) {
	var (
		s       Suggestion
		anonInt int
	)

	err := db.QueryRow(`
		SELECT
			id,
			user_id,
			username,
			display_name,
			text,
			anonymous,
			status,
			created_at,
			reviewed_by,
			review_message
		FROM suggestions
		WHERE id=?
	`,
		id,
	).Scan(
		&s.ID,
		&s.UserID,
		&s.Username,
		&s.DisplayName,
		&s.Text,
		&anonInt,
		&s.Status,
		&s.CreatedAt,
		&s.ReviewedBy,
		&s.ReviewMessage,
	)

	s.Anonymous = anonInt != 0

	return s, err
}

func sendQueue(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
) error {
	rows, err := db.Query(`
		SELECT
			id,
			text,
			anonymous,
			created_at
		FROM suggestions
		WHERE status='pending'
		ORDER BY id ASC
		LIMIT 20
	`)

	if err != nil {
		return err
	}

	defer rows.Close()

	var b strings.Builder

	b.WriteString(cfg.Texts.Admin.QueueTitle)
	b.WriteString("\n\n")

	count := 0

	for rows.Next() {
		var (
			id        int64
			text      string
			anonymous int
			createdAt time.Time
		)

		if err := rows.Scan(
			&id,
			&text,
			&anonymous,
			&createdAt,
		); err != nil {
			return err
		}

		count++

		b.WriteString(
			fmt.Sprintf(
				cfg.Texts.Admin.QueueItem,
				id,
				createdAt.Format("2006-01-02 15:04"),
				truncate(text, 120),
			),
		)

		b.WriteString("\n\n")
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if count == 0 {
		b.WriteString(cfg.Texts.Admin.QueueEmpty)
	}

	return api.SendMessage(
		ctx,
		cfg.AdminChatID,
		b.String(),
		nil,
	)
}

func (api *BotAPI) call(
	ctx context.Context,
	method string,
	payload any,
	result any,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.telegram.org/bot"+api.Token+"/"+method,
		bytes.NewReader(body),
	)

	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := api.Client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var envelope tgResponse[json.RawMessage]

	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf(
			"telegram returned invalid JSON: %w",
			err,
		)
	}

	if !envelope.OK {
		return fmt.Errorf(
			"telegram API: %s",
			envelope.Description,
		)
	}

	if result != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(
			envelope.Result,
			result,
		); err != nil {
			return err
		}
	}

	return nil
}

func (api *BotAPI) GetMe(
	ctx context.Context,
) (tgUser, error) {
	var user tgUser

	err := api.call(
		ctx,
		"getMe",
		map[string]any{},
		&user,
	)

	return user, err
}

func (api *BotAPI) GetUpdates(
	ctx context.Context,
	offset,
	limit,
	timeout int,
) ([]tgUpdate, error) {
	var updates []tgUpdate

	err := api.call(
		ctx,
		"getUpdates",
		map[string]any{
			"offset": offset,
			"limit":  limit,
			"timeout": timeout,
			"allowed_updates": []string{
				"message",
				"callback_query",
			},
		},
		&updates,
	)

	return updates, err
}

func (api *BotAPI) SendMessage(
	ctx context.Context,
	chatID int64,
	text string,
	replyMarkup any,
) error {
	_, err := api.SendMessageWithResult(
		ctx,
		chatID,
		text,
		replyMarkup,
	)

	return err
}

func (api *BotAPI) SendMessageWithResult(
	ctx context.Context,
	chatID int64,
	text string,
	replyMarkup any,
) (tgMessage, error) {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}

	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}

	var message tgMessage

	err := api.call(
		ctx,
		"sendMessage",
		payload,
		&message,
	)

	return message, err
}

func (api *BotAPI) AnswerCallbackQuery(
	ctx context.Context,
	callbackID,
	text string,
) error {
	return api.call(
		ctx,
		"answerCallbackQuery",
		map[string]any{
			"callback_query_id": callbackID,
			"text":              text,
		},
		nil,
	)
}

func userLabel(u tgUser) string {
	if u.Username != "" {
		return "@" + u.Username
	}

	name := strings.TrimSpace(
		strings.Join(
			[]string{
				u.FirstName,
				u.LastName,
			},
			" ",
		),
	)

	if name != "" {
		return name
	}

	return "пользователь"
}

func truncate(
	s string,
	n int,
) string {
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "…"
}

func boolInt(v bool) int {
	if v {
		return 1
	}

	return 0
}

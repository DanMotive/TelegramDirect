package main

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const albumCollectDelay = 900 * time.Millisecond

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
	AskText              string `json:"ask_text"`
	ModeAnonymous        string `json:"mode_anonymous"`
	ModeNamed            string `json:"mode_named"`
	ModeSelected         string `json:"mode_selected"`
	EmptyText            string `json:"empty_text"`
	TooLongText          string `json:"too_long_text"`
	SentText             string `json:"sent_text"`
	NoModeText           string `json:"no_mode_text"`
	CancelText           string `json:"cancel_text"`
	UnsupportedMediaText string `json:"unsupported_media_text"`
	AnonymousAuthor      string `json:"anonymous_author"`
	ReplyText            string `json:"reply_text"`
	MaxLength            int    `json:"max_length"`
}

type ModerationTextConfig struct {
	Title             string `json:"title"`
	IDLabel           string `json:"id_label"`
	AuthorLabel       string `json:"author_label"`
	TextLabel         string `json:"text_label"`
	MediaLabel        string `json:"media_label"`
	AnonymousAuthor   string `json:"anonymous_author"`
	AnonymousIDLabel  string `json:"anonymous_id_label"`
	NamedFallback     string `json:"named_fallback"`
	ApproveButton     string `json:"approve_button"`
	RejectButton      string `json:"reject_button"`
	ApprovedLog       string `json:"approved_log"`
	RejectedLog       string `json:"rejected_log"`
	AlreadyProcessed  string `json:"already_processed"`
	NotFound          string `json:"not_found"`
	InvalidID         string `json:"invalid_id"`
	NoPermission      string `json:"no_permission"`
	PublishedCallback string `json:"published_callback"`
	RejectedCallback  string `json:"rejected_callback"`
	ReplySentLog      string `json:"reply_sent_log"`
	ReplyNotFound     string `json:"reply_not_found"`
	ReplyEmpty        string `json:"reply_empty"`
	ReplyFailed       string `json:"reply_failed"`
}

type ChannelTextConfig struct {
	Header            string `json:"header"`
	ShowID            bool   `json:"show_id"`
	ShowAuthor        bool   `json:"show_author"`
	ShowAnonymousID   bool   `json:"show_anonymous_id"`
	AuthorPrefix      string `json:"author_prefix"`
	AnonymousAuthor   string `json:"anonymous_author"`
	AnonymousIDPrefix string `json:"anonymous_id_prefix"`
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

type tgPhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int    `json:"file_size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type tgVideo struct {
	FileID string `json:"file_id"`
}

type tgDocument struct {
	FileID string `json:"file_id"`
}

type tgAudio struct {
	FileID string `json:"file_id"`
}

type tgVoice struct {
	FileID string `json:"file_id"`
}

type tgVideoNote struct {
	FileID string `json:"file_id"`
}

type tgAnimation struct {
	FileID string `json:"file_id"`
}

type tgSticker struct {
	FileID string `json:"file_id"`
}

type tgMessage struct {
	MessageID      int        `json:"message_id"`
	From           *tgUser    `json:"from"`
	Chat           tgChat     `json:"chat"`
	Text           string     `json:"text"`
	Caption        string     `json:"caption"`
	MediaGroupID   string     `json:"media_group_id"`
	ReplyToMessage *tgMessage `json:"reply_to_message"`

	Photo     []tgPhotoSize `json:"photo"`
	Video     *tgVideo      `json:"video"`
	Document  *tgDocument   `json:"document"`
	Audio     *tgAudio      `json:"audio"`
	Voice     *tgVoice      `json:"voice"`
	VideoNote *tgVideoNote  `json:"video_note"`
	Animation *tgAnimation  `json:"animation"`
	Sticker   *tgSticker    `json:"sticker"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	From    tgUser     `json:"from"`
	Message *tgMessage `json:"message"`
	Data    string     `json:"data"`
}

type tgUpdate struct {
	UpdateID      int              `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}

type tgResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

type Suggestion struct {
	ID            int64
	UserID        int64
	Username      string
	DisplayName   string
	Text          string
	Anonymous     bool
	AnonymousID   string
	Status        string
	CreatedAt     time.Time
	ReviewedBy    int64
	ReviewMessage int
}

type pendingAlbum struct {
	Messages []*tgMessage
	Timer    *time.Timer
}

type AlbumCollector struct {
	mu     sync.Mutex
	albums map[string]*pendingAlbum
}

func NewAlbumCollector() *AlbumCollector {
	return &AlbumCollector{
		albums: make(map[string]*pendingAlbum),
	}
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
		Token: cfg.BotToken,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	ctx := context.Background()

	me, err := api.GetMe(ctx)
	if err != nil {
		log.Fatal("telegram authentication failed: ", err)
	}

	log.Printf(
		"bot started: @%s (%d)",
		me.Username,
		me.ID,
	)

	collector := NewAlbumCollector()

	var offset int

	for {
		updates, err := api.GetUpdates(
			ctx,
			offset,
			50,
			50,
		)

		if err != nil {
			log.Printf("polling error: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}

			if err := handleUpdate(
				ctx,
				api,
				db,
				cfg,
				collector,
				upd,
			); err != nil {
				log.Printf(
					"update %d failed: %v",
					upd.UpdateID,
					err,
				)
			}
		}
	}
}

func defaultTextConfig() TextConfig {
	return TextConfig{
		Bot: BotTextConfig{
			Name:            "TelegramDirect",
			WelcomeText:     "Hello! You can send a suggestion here.",
			HelpText:        "Choose how you want to submit your suggestion.",
			AnonymousButton: "🕵️ Anonymous",
			NamedButton:     "👤 With my name",
			CancelButton:    "❌ Cancel",
			UnknownCommand:  "Use /suggest to send a suggestion.",
		},

		Suggestion: SuggestionTextConfig{
			AskText:              "✍️ Now send your suggestion. Selected mode: %s.",
			ModeAnonymous:        "anonymous",
			ModeNamed:            "with my name",
			ModeSelected:         "Mode: %s",
			EmptyText:            "The suggestion cannot be empty.",
			TooLongText:          "The suggestion is too long. Maximum: %d characters.",
			SentText:             "✅ Your suggestion has been sent for moderation.",
			NoModeText:           "First choose a submission mode using /suggest.",
			CancelText:           "❌ Submission cancelled.",
			UnsupportedMediaText: "❌ This type of media is not supported. GIFs and stickers are not allowed.",
			AnonymousAuthor:      "Anonymous",
			ReplyText:            "💬 Reply to your suggestion #%d:\n\n%s",
			MaxLength:            4000,
		},

		Moderation: ModerationTextConfig{
			Title:             "📩 New Suggestion",
			IDLabel:           "ID",
			AuthorLabel:       "Author",
			TextLabel:         "Text",
			MediaLabel:        "Attachments",
			AnonymousAuthor:   "Anonymous",
			AnonymousIDLabel:  "Anonymous ID",
			NamedFallback:     "User without username",
			ApproveButton:     "✅ Publish",
			RejectButton:      "❌ Reject",
			ApprovedLog:       "✅ Suggestion #%d was published by %s (ID %d).",
			RejectedLog:       "❌ Suggestion #%d was rejected by %s (ID %d).",
			AlreadyProcessed:  "This suggestion has already been processed.",
			NotFound:          "Suggestion not found.",
			InvalidID:         "Invalid suggestion ID.",
			NoPermission:      "You do not have permission to do this.",
			PublishedCallback: "Published",
			RejectedCallback:  "Rejected",
			ReplySentLog:      "💬 Reply to suggestion #%d was sent to %s by %s.",
			ReplyNotFound:     "This message is not linked to a suggestion.",
			ReplyEmpty:        "The reply cannot be empty.",
			ReplyFailed:       "Failed to send the reply.",
		},

		Channel: ChannelTextConfig{
			Header:            "📢 Suggestion",
			ShowID:            false,
			ShowAuthor:        true,
			ShowAnonymousID:   false,
			AuthorPrefix:      "👤 Author:",
			AnonymousAuthor:   "Anonymous",
			AnonymousIDPrefix: "🆔",
		},

		Admin: AdminTextConfig{
			QueueTitle:   "📋 Suggestion Queue",
			QueueItem:    "#%d — %s\n%s",
			QueueEmpty:   "The queue is empty.",
			HelpText:     "Commands:\n/queue — show the suggestion queue\n/help — show this help message",
			PublishedLog: "✅ Suggestion #%d was published by %s (ID %d).",
			RejectedLog:  "❌ Suggestion #%d was rejected by %s (ID %d).",
		},

		Errors: ErrorTextConfig{
			Generic:       "Something went wrong. Please try again.",
			NotAllowed:    "You do not have access to this action.",
			Unavailable:   "This message is no longer available.",
			InvalidAction: "Invalid action.",
			InvalidMode:   "Invalid submission mode.",
		},
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
		return Config{}, fmt.Errorf(
			"invalid ADMIN_CHAT_ID: %w",
			err,
		)
	}

	channelID, err := strconv.ParseInt(
		strings.TrimSpace(os.Getenv("CHANNEL_ID")),
		10,
		64,
	)
	if err != nil {
		return Config{}, fmt.Errorf(
			"invalid CHANNEL_ID: %w",
			err,
		)
	}

	adminIDs := make(map[int64]bool)

	for _, raw := range strings.Split(
		os.Getenv("ADMIN_IDS"),
		",",
	) {
		raw = strings.TrimSpace(raw)

		if raw == "" {
			continue
		}

		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf(
				"invalid admin ID %q: %w",
				raw,
				err,
			)
		}

		adminIDs[id] = true
	}

	if len(adminIDs) == 0 {
		return Config{}, errors.New(
			"ADMIN_IDS must contain at least one Telegram user ID",
		)
	}

	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		dbPath = "data.db"
	}

	configPath := strings.TrimSpace(
		os.Getenv("CONFIG_PATH"),
	)
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
	cfg := defaultTextConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return TextConfig{}, fmt.Errorf(
			"failed to read config %s: %w",
			path,
			err,
		)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return TextConfig{}, fmt.Errorf(
			"invalid JSON config %s: %w",
			path,
			err,
		)
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
			anonymous_id TEXT NOT NULL DEFAULT '',
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

		CREATE TABLE IF NOT EXISTS anonymous_users (
			user_id INTEGER PRIMARY KEY,
			anonymous_id TEXT NOT NULL UNIQUE
		);

		CREATE TABLE IF NOT EXISTS suggestion_media (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			suggestion_id INTEGER NOT NULL,
			source_chat_id INTEGER NOT NULL,
			source_message_id INTEGER NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(suggestion_id) REFERENCES suggestions(id)
		);

		CREATE INDEX IF NOT EXISTS idx_suggestion_media_suggestion
		ON suggestion_media(suggestion_id);

		CREATE TABLE IF NOT EXISTS suggestion_messages (
			message_id INTEGER PRIMARY KEY,
			suggestion_id INTEGER NOT NULL,
			FOREIGN KEY(suggestion_id) REFERENCES suggestions(id)
		);

		CREATE INDEX IF NOT EXISTS idx_suggestion_messages_suggestion
		ON suggestion_messages(suggestion_id);
	`)

	if err != nil {
		return err
	}

	var columnCount int

	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('suggestions')
		WHERE name='anonymous_id'
	`).Scan(&columnCount)

	if err != nil {
		return err
	}

	if columnCount == 0 {
		if _, err := db.Exec(`
			ALTER TABLE suggestions
			ADD COLUMN anonymous_id TEXT NOT NULL DEFAULT ''
		`); err != nil {
			return err
		}
	}

	return migrateAnonymousIDs(db)
}

func migrateAnonymousIDs(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT DISTINCT user_id
		FROM suggestions
		WHERE anonymous=1
		AND anonymous_id=''
	`)
	if err != nil {
		return err
	}

	defer rows.Close()

	var userIDs []int64

	for rows.Next() {
		var userID int64

		if err := rows.Scan(&userID); err != nil {
			return err
		}

		userIDs = append(userIDs, userID)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	for _, userID := range userIDs {
		anonymousID, err := ensureAnonymousID(
			db,
			userID,
		)

		if err != nil {
			return err
		}

		_, err = db.Exec(`
			UPDATE suggestions
			SET anonymous_id=?
			WHERE user_id=?
			AND anonymous=1
			AND anonymous_id=''
		`,
			anonymousID,
			userID,
		)

		if err != nil {
			return err
		}
	}

	return nil
}

func handleUpdate(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	collector *AlbumCollector,
	upd tgUpdate,
) error {
	if upd.CallbackQuery != nil {
		return handleCallback(
			ctx,
			api,
			db,
			cfg,
			upd.CallbackQuery,
		)
	}

	if upd.Message != nil {
		return handleMessage(
			ctx,
			api,
			db,
			cfg,
			collector,
			upd.Message,
		)
	}

	return nil
}

func handleMessage(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	collector *AlbumCollector,
	msg *tgMessage,
) error {
	if msg.From == nil || msg.From.IsBot {
		return nil
	}

	if msg.Chat.ID == cfg.AdminChatID &&
		cfg.AdminIDs[msg.From.ID] &&
		msg.ReplyToMessage != nil {

		return handleAdminReply(
			ctx,
			api,
			db,
			cfg,
			msg,
		)
	}

	if msg.Chat.ID == cfg.AdminChatID &&
		cfg.AdminIDs[msg.From.ID] {

		return handleAdminMessage(
			ctx,
			api,
			db,
			cfg,
			msg,
		)
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

	case "/help":
		return api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Bot.HelpText,
			nil,
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

	if hasUnsupportedMedia(msg) {
		return api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Suggestion.UnsupportedMediaText,
			nil,
		)
	}

	if msg.MediaGroupID != "" {
		key := fmt.Sprintf(
			"%d:%s",
			msg.Chat.ID,
			msg.MediaGroupID,
		)

		return collector.Add(
			key,
			msg,
			func(messages []*tgMessage) {
				flushAlbum(
					ctx,
					api,
					db,
					cfg,
					messages,
				)
			},
		)
	}

	if hasSupportedMedia(msg) {
		return submitMediaSuggestion(
			ctx,
			api,
			db,
			cfg,
			[]*tgMessage{msg},
		)
	}

	return submitTextSuggestion(
		ctx,
		api,
		db,
		cfg,
		*msg.From,
		msg.Text,
	)
}

func flushAlbum(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	messages []*tgMessage,
) {
	if len(messages) == 0 {
		return
	}

	for _, msg := range messages {
		if hasUnsupportedMedia(msg) {
			_ = api.SendMessage(
				ctx,
				msg.Chat.ID,
				cfg.Texts.Suggestion.UnsupportedMediaText,
				nil,
			)
			return
		}
	}

	if err := submitMediaSuggestion(
		ctx,
		api,
		db,
		cfg,
		messages,
	); err != nil {
		log.Printf(
			"album submission failed: %v",
			err,
		)
	}
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

func handleAdminReply(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	msg *tgMessage,
) error {
	if msg.ReplyToMessage == nil {
		return nil
	}

	var (
		suggestionID int64
		userID       int64
	)

	err := db.QueryRow(`
		SELECT
			s.id,
			s.user_id
		FROM suggestion_messages sm
		JOIN suggestions s
			ON s.id = sm.suggestion_id
		WHERE sm.message_id=?
	`,
		msg.ReplyToMessage.MessageID,
	).Scan(
		&suggestionID,
		&userID,
	)

	if err == sql.ErrNoRows {
		return api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Moderation.ReplyNotFound,
			nil,
		)
	}

	if err != nil {
		return err
	}

	if hasUnsupportedMedia(msg) {
		return api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Suggestion.UnsupportedMediaText,
			nil,
		)
	}

	if !hasMessageContent(msg) {
		return api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Moderation.ReplyEmpty,
			nil,
		)
	}

	_, err = api.CopyMessage(
		ctx,
		userID,
		msg.Chat.ID,
		msg.MessageID,
	)

	if err != nil {
		_ = api.SendMessage(
			ctx,
			msg.Chat.ID,
			cfg.Texts.Moderation.ReplyFailed,
			nil,
		)

		return err
	}

	identifier, err := getSuggestionIdentifier(
		db,
		suggestionID,
	)

	if err != nil {
		identifier = "user"
	}

	logText := fmt.Sprintf(
		cfg.Texts.Moderation.ReplySentLog,
		suggestionID,
		identifier,
		userLabel(*msg.From),
	)

	return api.SendMessage(
		ctx,
		msg.Chat.ID,
		logText,
		nil,
	)
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
		`,
			cb.From.ID,
			mode,
		)

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
			fmt.Sprintf(
				cfg.Texts.Suggestion.ModeSelected,
				label,
			),
		); err != nil {
			return err
		}

		return api.SendMessage(
			ctx,
			cb.From.ID,
			fmt.Sprintf(
				cfg.Texts.Suggestion.AskText,
				label,
			),
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

func submitTextSuggestion(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	user tgUser,
	text string,
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

	mode, err := getUserMode(db, user.ID)
	if err != nil {
		return err
	}

	anonymous := mode == 1

	anonymousID := ""

	if anonymous {
		anonymousID, err = ensureAnonymousID(
			db,
			user.ID,
		)
		if err != nil {
			return err
		}
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

	result, err := db.Exec(`
		INSERT INTO suggestions
		(
			user_id,
			username,
			display_name,
			text,
			anonymous,
			anonymous_id
		)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		user.ID,
		user.Username,
		displayName,
		text,
		boolInt(anonymous),
		anonymousID,
	)

	if err != nil {
		return err
	}

	suggestionID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	s, err := getSuggestion(
		db,
		suggestionID,
	)
	if err != nil {
		return err
	}

	reviewMessageID, err := sendModerationMessage(
		ctx,
		api,
		cfg,
		s,
	)

	if err != nil {
		return err
	}

	if _, err := db.Exec(`
		UPDATE suggestions
		SET review_message=?
		WHERE id=?
	`,
		reviewMessageID,
		suggestionID,
	); err != nil {
		return err
	}

	if _, err := db.Exec(`
		INSERT OR REPLACE INTO suggestion_messages
		(message_id, suggestion_id)
		VALUES (?, ?)
	`,
		reviewMessageID,
		suggestionID,
	); err != nil {
		return err
	}

	if _, err := db.Exec(`
		DELETE FROM user_states
		WHERE user_id=?
	`,
		user.ID,
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

func submitMediaSuggestion(
	ctx context.Context,
	api *BotAPI,
	db *sql.DB,
	cfg Config,
	messages []*tgMessage,
) error {
	if len(messages) == 0 {
		return nil
	}

	user := messages[0].From

	if user == nil {
		return errors.New("media message has no author")
	}

	var textBuilder strings.Builder

	for _, msg := range messages {
		if msg.Caption != "" {
			if textBuilder.Len() > 0 {
				textBuilder.WriteString("\n\n")
			}

			textBuilder.WriteString(strings.TrimSpace(msg.Caption))
		}
	}

	text := textBuilder.String()

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

	mode, err := getUserMode(
		db,
		user.ID,
	)
	if err != nil {
		return err
	}

	anonymous := mode == 1

	anonymousID := ""

	if anonymous {
		anonymousID, err = ensureAnonymousID(
			db,
			user.ID,
		)

		if err != nil {
			return err
		}
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

	result, err := db.Exec(`
		INSERT INTO suggestions
		(
			user_id,
			username,
			display_name,
			text,
			anonymous,
			anonymous_id
		)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		user.ID,
		user.Username,
		displayName,
		text,
		boolInt(anonymous),
		anonymousID,
	)

	if err != nil {
		return err
	}

	suggestionID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	s, err := getSuggestion(
		db,
		suggestionID,
	)
	if err != nil {
		return err
	}

	reviewMessageID, err := sendModerationMessage(
		ctx,
		api,
		cfg,
		s,
	)
	if err != nil {
		return err
	}

	if _, err := db.Exec(`
		UPDATE suggestions
		SET review_message=?
		WHERE id=?
	`,
		reviewMessageID,
		suggestionID,
	); err != nil {
		return err
	}

	if _, err := db.Exec(`
		INSERT OR REPLACE INTO suggestion_messages
		(message_id, suggestion_id)
		VALUES (?, ?)
	`,
		reviewMessageID,
		suggestionID,
	); err != nil {
		return err
	}

	for index, msg := range messages {
		if _, err := db.Exec(`
			INSERT INTO suggestion_media
			(
				suggestion_id,
				source_chat_id,
				source_message_id,
				sort_order
			)
			VALUES (?, ?, ?, ?)
		`,
			suggestionID,
			msg.Chat.ID,
			msg.MessageID,
			index,
		); err != nil {
			return err
		}
	}

	for _, msg := range messages {
		// The copied moderation media message must also be mapped
		// so an administrator can reply to any attachment.
		copied, err := api.CopyMessage(
			ctx,
			cfg.AdminChatID,
			msg.Chat.ID,
			msg.MessageID,
		)

		if err != nil {
			return err
		}

		if _, err := db.Exec(`
			INSERT OR REPLACE INTO suggestion_messages
			(message_id, suggestion_id)
			VALUES (?, ?)
		`,
			copied.MessageID,
			suggestionID,
		); err != nil {
			return err
		}
	}

	if _, err := db.Exec(`
		DELETE FROM user_states
		WHERE user_id=?
	`,
		user.ID,
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

func getUserMode(
	db *sql.DB,
	userID int64,
) (int, error) {
	var mode int

	err := db.QueryRow(`
		SELECT mode
		FROM user_states
		WHERE user_id=?
	`,
		userID,
	).Scan(&mode)

	if err == sql.ErrNoRows {
		return -1, errors.New(
			"user has not selected a submission mode",
		)
	}

	return mode, err
}

func sendModerationMessage(
	ctx context.Context,
	api *BotAPI,
	cfg Config,
	s Suggestion,
) (int, error) {
	author := formatSuggestionAuthor(
		s,
		cfg.Texts.Moderation.AnonymousAuthor,
		cfg.Texts.Moderation.NamedFallback,
	)

	var b strings.Builder

	b.WriteString(cfg.Texts.Moderation.Title)
	b.WriteString("\n\n")

	b.WriteString(
		fmt.Sprintf(
			"%s: #%d\n",
			cfg.Texts.Moderation.IDLabel,
			s.ID,
		),
	)

	b.WriteString(
		fmt.Sprintf(
			"%s: %s\n",
			cfg.Texts.Moderation.AuthorLabel,
			author,
		),
	)

	if s.Anonymous && s.AnonymousID != "" {
		b.WriteString(
			fmt.Sprintf(
				"%s: %s\n",
				cfg.Texts.Moderation.AnonymousIDLabel,
				s.AnonymousID,
			),
		)
	}

	if s.Text != "" {
		b.WriteString("\n")
		b.WriteString(
			cfg.Texts.Moderation.TextLabel,
		)
		b.WriteString(":\n")
		b.WriteString(s.Text)
	}

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
		b.String(),
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
	id, err := strconv.ParseInt(
		rawID,
		10,
		64,
	)

	if err != nil {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Moderation.InvalidID,
		)
	}

	s, err := getSuggestion(
		db,
		id,
	)

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

	media, err := getSuggestionMedia(
		db,
		id,
	)

	if err != nil {
		return err
	}

	if len(media) == 0 {
		channelText := buildChannelMessage(
			cfg,
			s,
		)

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
	} else {
		header := buildChannelHeader(
			cfg,
			s,
		)

		if err := api.SendMessage(
			ctx,
			cfg.ChannelID,
			header,
			nil,
		); err != nil {
			return api.AnswerCallbackQuery(
				ctx,
				cb.ID,
				cfg.Texts.Errors.Generic,
			)
		}

		messageIDs := make([]int, 0, len(media))

		for _, item := range media {
			messageIDs = append(
				messageIDs,
				item.SourceMessageID,
			)
		}

		if _, err := api.CopyMessages(
			ctx,
			cfg.ChannelID,
			media[0].SourceChatID,
			messageIDs,
		); err != nil {
			return api.AnswerCallbackQuery(
				ctx,
				cb.ID,
				cfg.Texts.Errors.Generic,
			)
		}
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
	id, err := strconv.ParseInt(
		rawID,
		10,
		64,
	)

	if err != nil {
		return api.AnswerCallbackQuery(
			ctx,
			cb.ID,
			cfg.Texts.Moderation.InvalidID,
		)
	}

	s, err := getSuggestion(
		db,
		id,
	)

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

type suggestionMedia struct {
	SourceChatID    int64
	SourceMessageID int
	SortOrder       int
}

func getSuggestionMedia(
	db *sql.DB,
	suggestionID int64,
) ([]suggestionMedia, error) {
	rows, err := db.Query(`
		SELECT
			source_chat_id,
			source_message_id,
			sort_order
		FROM suggestion_media
		WHERE suggestion_id=?
		ORDER BY sort_order ASC
	`,
		suggestionID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var media []suggestionMedia

	for rows.Next() {
		var item suggestionMedia

		if err := rows.Scan(
			&item.SourceChatID,
			&item.SourceMessageID,
			&item.SortOrder,
		); err != nil {
			return nil, err
		}

		media = append(
			media,
			item,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return media, nil
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
			anonymous_id,
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
		&s.AnonymousID,
		&s.Status,
		&s.CreatedAt,
		&s.ReviewedBy,
		&s.ReviewMessage,
	)

	s.Anonymous = anonInt != 0

	return s, err
}

func getSuggestionIdentifier(
	db *sql.DB,
	id int64,
) (string, error) {
	s, err := getSuggestion(
		db,
		id,
	)

	if err != nil {
		return "", err
	}

	if s.Anonymous {
		if s.AnonymousID != "" {
			return s.AnonymousID, nil
		}

		return "Anonymous", nil
	}

	if s.Username != "" {
		return "@" + s.Username, nil
	}

	if s.DisplayName != "" {
		return s.DisplayName, nil
	}

	return "user", nil
}

func buildChannelHeader(
	cfg Config,
	s Suggestion,
) string {
	var b strings.Builder

	if cfg.Texts.Channel.Header != "" {
		b.WriteString(cfg.Texts.Channel.Header)
		b.WriteString("\n\n")
	}

	if cfg.Texts.Channel.ShowID {
		b.WriteString(
			fmt.Sprintf("#%d", s.ID),
		)
		b.WriteString("\n\n")
	}

	if cfg.Texts.Channel.ShowAuthor {
		b.WriteString(
			cfg.Texts.Channel.AuthorPrefix,
		)
		b.WriteString(" ")

		if s.Anonymous {
			b.WriteString(
				cfg.Texts.Channel.AnonymousAuthor,
			)
		} else if s.Username != "" {
			b.WriteString("@")
			b.WriteString(s.Username)
		} else if s.DisplayName != "" {
			b.WriteString(s.DisplayName)
		}

		if s.Anonymous &&
			cfg.Texts.Channel.ShowAnonymousID &&
			s.AnonymousID != "" {

			b.WriteString("\n")
			b.WriteString(
				cfg.Texts.Channel.AnonymousIDPrefix,
			)
			b.WriteString(" ")
			b.WriteString(s.AnonymousID)
		}
	}

	return strings.TrimSpace(b.String())
}

func buildChannelMessage(
	cfg Config,
	s Suggestion,
) string {
	header := buildChannelHeader(
		cfg,
		s,
	)

	if header == "" {
		return s.Text
	}

	if s.Text == "" {
		return header
	}

	return header + "\n\n" + s.Text
}

func formatSuggestionAuthor(
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

	return "user"
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

	b.WriteString(
		cfg.Texts.Admin.QueueTitle,
	)
	b.WriteString("\n\n")

	count := 0

	for rows.Next() {
		var (
			id        int64
			text      string
			createdAt time.Time
		)

		if err := rows.Scan(
			&id,
			&text,
			&createdAt,
		); err != nil {
			return err
		}

		count++

		if text == "" {
			text = cfg.Texts.Moderation.MediaLabel
		}

		b.WriteString(
			fmt.Sprintf(
				cfg.Texts.Admin.QueueItem,
				id,
				createdAt.Format(
					"2006-01-02 15:04",
				),
				truncate(text, 120),
			),
		)

		b.WriteString("\n\n")
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if count == 0 {
		b.WriteString(
			cfg.Texts.Admin.QueueEmpty,
		)
	}

	return api.SendMessage(
		ctx,
		cfg.AdminChatID,
		b.String(),
		nil,
	)
}

func ensureAnonymousID(
	db *sql.DB,
	userID int64,
) (string, error) {
	var anonymousID string

	err := db.QueryRow(`
		SELECT anonymous_id
		FROM anonymous_users
		WHERE user_id=?
	`,
		userID,
	).Scan(&anonymousID)

	if err == nil {
		return anonymousID, nil
	}

	if err != sql.ErrNoRows {
		return "", err
	}

	for i := 0; i < 20; i++ {
		candidate, err := generateAnonymousID()
		if err != nil {
			return "", err
		}

		_, err = db.Exec(`
			INSERT INTO anonymous_users
			(user_id, anonymous_id)
			VALUES (?, ?)
		`,
			userID,
			candidate,
		)

		if err == nil {
			return candidate, nil
		}

		if strings.Contains(
			err.Error(),
			"UNIQUE constraint failed",
		) {
			var existing string

			queryErr := db.QueryRow(`
				SELECT anonymous_id
				FROM anonymous_users
				WHERE user_id=?
			`,
				userID,
			).Scan(&existing)

			if queryErr == nil {
				return existing, nil
			}
		}
	}

	return "", errors.New(
		"failed to generate unique anonymous ID",
	)
}

func generateAnonymousID() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	buf := make([]byte, 6)

	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	var b strings.Builder

	b.WriteString("ANON-")

	for _, value := range buf {
		b.WriteByte(
			alphabet[int(value)%len(alphabet)],
		)
	}

	return b.String(), nil
}

func hasSupportedMedia(msg *tgMessage) bool {
	return len(msg.Photo) > 0 ||
		msg.Video != nil ||
		msg.Document != nil ||
		msg.Audio != nil ||
		msg.Voice != nil ||
		msg.VideoNote != nil
}

func hasUnsupportedMedia(msg *tgMessage) bool {
	return msg.Animation != nil ||
		msg.Sticker != nil
}

func hasMessageContent(msg *tgMessage) bool {
	return strings.TrimSpace(msg.Text) != "" ||
		strings.TrimSpace(msg.Caption) != "" ||
		hasSupportedMedia(msg) ||
		hasUnsupportedMedia(msg)
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

	return "user"
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

func (c *AlbumCollector) Add(
	key string,
	msg *tgMessage,
	flush func([]*tgMessage),
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if album, ok := c.albums[key]; ok {
		album.Messages = append(
			album.Messages,
			msg,
		)

		if album.Timer != nil {
			album.Timer.Stop()
		}

		album.Timer = time.AfterFunc(
			albumCollectDelay,
			func() {
				c.flush(key, flush)
			},
		)

		return nil
	}

	album := &pendingAlbum{
		Messages: []*tgMessage{msg},
	}

	album.Timer = time.AfterFunc(
		albumCollectDelay,
		func() {
			c.flush(key, flush)
		},
	)

	c.albums[key] = album

	return nil
}

func (c *AlbumCollector) flush(
	key string,
	flush func([]*tgMessage),
) {
	c.mu.Lock()

	album, ok := c.albums[key]
	if !ok {
		c.mu.Unlock()
		return
	}

	delete(c.albums, key)

	messages := append(
		[]*tgMessage(nil),
		album.Messages...,
	)

	c.mu.Unlock()

	flush(messages)
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
		"https://api.telegram.org/bot"+
			api.Token+
			"/"+
			method,
		bytes.NewReader(body),
	)

	if err != nil {
		return err
	}

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

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

	if err := json.Unmarshal(
		data,
		&envelope,
	); err != nil {
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

	if result != nil &&
		len(envelope.Result) > 0 {

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
			"offset":  offset,
			"limit":   limit,
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

func (api *BotAPI) CopyMessage(
	ctx context.Context,
	chatID int64,
	fromChatID int64,
	messageID int,
) (tgMessage, error) {
	var message tgMessage

	err := api.call(
		ctx,
		"copyMessage",
		map[string]any{
			"chat_id":      chatID,
			"from_chat_id": fromChatID,
			"message_id":   messageID,
		},
		&message,
	)

	return message, err
}

func (api *BotAPI) CopyMessages(
	ctx context.Context,
	chatID int64,
	fromChatID int64,
	messageIDs []int,
) ([]tgMessage, error) {
	var messages []tgMessage

	err := api.call(
		ctx,
		"copyMessages",
		map[string]any{
			"chat_id":      chatID,
			"from_chat_id": fromChatID,
			"message_ids":  messageIDs,
		},
		&messages,
	)

	return messages, err
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

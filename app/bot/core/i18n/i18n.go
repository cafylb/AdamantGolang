package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

const DefaultLanguage = "ru"

type segment struct {
	value       string
	placeholder bool
}

type template struct {
	raw      string
	segments []segment
}

type catalogState struct {
	messages map[string]map[string]*template
	langs    []string
}

type Localizer struct {
	lang  string
	state *catalogState
}

type Vars = map[string]any

type Message struct {
	key  string
	text *template
}

var (
	currentLang  atomic.Value
	currentState atomic.Value
	userLangs    sync.Map
)

func init() {
	currentLang.Store(DefaultLanguage)
	currentState.Store(&catalogState{
		messages: map[string]map[string]*template{},
	})
}

func Init() error {
	return Load(defaultFiles())
}

func MustInit() {
	if err := Init(); err != nil {
		panic(err)
	}
}

func Load(files map[string]string) error {
	if len(files) == 0 {
		return errors.New("i18n: files map is empty")
	}

	messages := make(map[string]map[string]*template, len(files))
	langs := make([]string, 0, len(files))

	for lang, path := range files {
		normalized := normalizeLanguage(lang)
		if normalized == "" {
			return fmt.Errorf("i18n: empty language code for %q", path)
		}
		if _, exists := messages[normalized]; exists {
			return fmt.Errorf("i18n: duplicate language %q", normalized)
		}

		catalog, err := loadCatalog(path)
		if err != nil {
			return fmt.Errorf("i18n: load %s from %s: %w", normalized, path, err)
		}

		messages[normalized] = catalog
		langs = append(langs, normalized)
	}

	sort.Strings(langs)

	state := &catalogState{
		messages: messages,
		langs:    langs,
	}

	currentState.Store(state)
	currentLang.Store(matchLanguage(DefaultLanguage, state))

	return nil
}

func Get(key string, args ...any) string {
	return Current().Get(key).Format(args...)
}

func GetFor(lang, key string, args ...any) string {
	return For(lang).Get(key).Format(args...)
}

func Lookup(key string) Message {
	return Current().Get(key)
}

func LookupFor(lang, key string) Message {
	return For(lang).Get(key)
}

func For(lang string) Localizer {
	state := snapshot()
	return Localizer{
		lang:  matchLanguage(lang, state),
		state: state,
	}
}

func Current() Localizer {
	return For(Language())
}

func ForUser(userID int64) Localizer {
	state := snapshot()
	return Localizer{
		lang:  resolveUserLanguage(userID, state),
		state: state,
	}
}

func UserLanguage(userID int64) string {
	return resolveUserLanguage(userID, snapshot())
}

func SetUserLanguage(userID int64, lang string) string {
	next := matchLanguage(lang, snapshot())
	if userID != 0 {
		userLangs.Store(userID, next)
	}
	return next
}

func ResetUserLanguage(userID int64) {
	if userID != 0 {
		userLangs.Delete(userID)
	}
}

func ChangeLanguage(lang string) string {
	next := matchLanguage(lang, snapshot())
	currentLang.Store(next)
	return next
}

func Language() string {
	value, _ := currentLang.Load().(string)
	if value == "" {
		return DefaultLanguage
	}
	return value
}

func Languages() []string {
	langs := snapshot().langs
	out := make([]string, len(langs))
	copy(out, langs)
	return out
}

func HasLanguage(lang string) bool {
	normalized := normalizeLanguage(lang)
	if normalized == "" {
		return false
	}

	_, ok := snapshot().messages[normalized]
	return ok
}

func (l Localizer) Language() string {
	if l.lang == "" {
		return DefaultLanguage
	}
	return l.lang
}

func (l Localizer) Get(key string) Message {
	if text, ok := lookup(l.state, l.lang, key); ok {
		return Message{
			key:  key,
			text: text,
		}
	}

	return Message{key: key}
}

func (l Localizer) Main(key string) Message {
	key = strings.TrimPrefix(key, "main.")
	return l.Get("main." + key)
}

func (l Localizer) Button(key string) Message {
	key = strings.TrimPrefix(key, "button.")
	return l.Get("button." + key)
}

func (l Localizer) Payment(key string) Message {
	key = strings.TrimPrefix(key, "payment.")
	return l.Get("payment." + key)
}

func (l Localizer) Error(key string) Message {
	key = strings.TrimPrefix(key, "error.")
	return l.Get("error." + key)
}

func (m Message) String() string {
	if m.text == nil {
		return m.key
	}
	return m.text.raw
}

func (m Message) Raw() string {
	return m.String()
}

func (m Message) Format(args ...any) string {
	if m.text == nil {
		return m.key
	}
	return m.text.format(args...)
}

func (m Message) FormatMap(values map[string]any) string {
	if m.text == nil {
		return m.key
	}
	if len(values) == 0 {
		return m.text.raw
	}
	return m.text.format(values)
}

func resolveUserLanguage(userID int64, state *catalogState) string {
	if userID != 0 {
		if value, ok := userLangs.Load(userID); ok {
			if lang, ok := value.(string); ok && lang != "" {
				return matchLanguage(lang, state)
			}
		}
	}

	return matchLanguage(DefaultLanguage, state)
}

func snapshot() *catalogState {
	state, _ := currentState.Load().(*catalogState)
	if state == nil {
		return &catalogState{messages: map[string]map[string]*template{}}
	}
	return state
}

func lookup(state *catalogState, lang, key string) (*template, bool) {
	if state == nil || key == "" || len(state.messages) == 0 {
		return nil, false
	}

	if normalized := normalizeLanguage(lang); normalized != "" {
		if catalog, ok := state.messages[normalized]; ok {
			if value, found := catalog[key]; found {
				return value, true
			}
		}

		if base := baseLanguage(normalized); base != normalized {
			if catalog, ok := state.messages[base]; ok {
				if value, found := catalog[key]; found {
					return value, true
				}
			}
		}
	}

	if catalog, ok := state.messages[DefaultLanguage]; ok {
		if value, found := catalog[key]; found {
			return value, true
		}
	}

	for _, lang := range state.langs {
		if catalog, ok := state.messages[lang]; ok {
			if value, found := catalog[key]; found {
				return value, true
			}
		}
	}

	return nil, false
}

func matchLanguage(lang string, state *catalogState) string {
	if state == nil || len(state.messages) == 0 {
		return DefaultLanguage
	}

	normalized := normalizeLanguage(lang)
	if normalized != "" {
		if _, ok := state.messages[normalized]; ok {
			return normalized
		}

		if base := baseLanguage(normalized); base != normalized {
			if _, ok := state.messages[base]; ok {
				return base
			}
		}
	}

	if _, ok := state.messages[DefaultLanguage]; ok {
		return DefaultLanguage
	}

	return state.langs[0]
}

func loadCatalog(path string) (map[string]*template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]*template{}, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	flat := make(map[string]*template)
	if err := flatten("", raw, flat); err != nil {
		return nil, err
	}

	registerAliases(flat)

	return flat, nil
}

func flatten(prefix string, raw map[string]any, dst map[string]*template) error {
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch value := value.(type) {
		case string:
			dst[fullKey] = compileTemplate(value)
		case float64:
			dst[fullKey] = compileTemplate(fmt.Sprint(value))
		case bool:
			dst[fullKey] = compileTemplate(fmt.Sprint(value))
		case nil:
			dst[fullKey] = compileTemplate("")
		case map[string]any:
			if err := flatten(fullKey, value, dst); err != nil {
				return err
			}
		default:
			return fmt.Errorf("i18n: unsupported value type %T for key %q", value, fullKey)
		}
	}

	return nil
}

func registerAliases(catalog map[string]*template) {
	if len(catalog) == 0 {
		return
	}

	aliases := make(map[string]*template, len(catalog))
	for key, value := range catalog {
		for _, alias := range keyAliases(key) {
			if alias == "" || alias == key {
				continue
			}
			if _, exists := catalog[alias]; exists {
				continue
			}
			if _, exists := aliases[alias]; exists {
				continue
			}
			aliases[alias] = value
		}
	}

	for alias, value := range aliases {
		catalog[alias] = value
	}
}

func keyAliases(key string) []string {
	switch {
	case strings.HasPrefix(key, "menu.buy_list."):
		tail := strings.TrimPrefix(key, "menu.buy_list.")
		return []string{
			"main." + tail,
			"main.buy_list." + tail,
		}
	case strings.HasPrefix(key, "menu."):
		return []string{"main." + strings.TrimPrefix(key, "menu.")}
	}

	return nil
}

func compileTemplate(raw string) *template {
	if raw == "" || !strings.Contains(raw, "{") {
		return &template{raw: raw}
	}

	segments := make([]segment, 0, 4)
	rest := raw

	for len(rest) > 0 {
		start := strings.IndexByte(rest, '{')
		if start == -1 {
			segments = append(segments, segment{value: rest})
			break
		}

		if start > 0 {
			segments = append(segments, segment{value: rest[:start]})
		}

		rest = rest[start+1:]

		end := strings.IndexByte(rest, '}')
		if end == -1 {
			segments = append(segments, segment{value: "{" + rest})
			break
		}

		key := rest[:end]
		if key == "" {
			segments = append(segments, segment{value: "{}"})
		} else {
			segments = append(segments, segment{
				value:       key,
				placeholder: true,
			})
		}

		rest = rest[end+1:]
	}

	if len(segments) == 1 && !segments[0].placeholder && segments[0].value == raw {
		return &template{raw: raw}
	}

	return &template{
		raw:      raw,
		segments: segments,
	}
}

func (t *template) format(args ...any) string {
	if t == nil {
		return ""
	}
	if len(t.segments) == 0 || len(args) == 0 {
		return t.raw
	}

	lookup, ok := buildLookup(args)
	if !ok {
		return t.raw
	}

	var builder strings.Builder
	builder.Grow(len(t.raw))

	for _, segment := range t.segments {
		if !segment.placeholder {
			builder.WriteString(segment.value)
			continue
		}

		if value, found := lookup(segment.value); found {
			builder.WriteString(value)
			continue
		}

		builder.WriteByte('{')
		builder.WriteString(segment.value)
		builder.WriteByte('}')
	}

	return builder.String()
}

func buildLookup(args []any) (func(string) (string, bool), bool) {
	if len(args) == 1 {
		switch values := args[0].(type) {
		case map[string]any:
			if len(values) == 0 {
				return nil, false
			}
			return func(key string) (string, bool) {
				value, ok := values[key]
				if !ok {
					return "", false
				}
				return fmt.Sprint(value), true
			}, true
		case map[string]string:
			if len(values) == 0 {
				return nil, false
			}
			return func(key string) (string, bool) {
				value, ok := values[key]
				return value, ok
			}, true
		}
	}

	if len(args) < 2 {
		return nil, false
	}

	return func(key string) (string, bool) {
		for i := 0; i+1 < len(args); i += 2 {
			name, ok := args[i].(string)
			if !ok || name != key {
				continue
			}
			return fmt.Sprint(args[i+1]), true
		}
		return "", false
	}, true
}

func normalizeLanguage(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(lang))
	if lang == "" {
		return ""
	}
	return strings.ReplaceAll(lang, "_", "-")
}

func baseLanguage(lang string) string {
	if head, _, ok := strings.Cut(lang, "-"); ok {
		return head
	}
	return lang
}

func defaultFiles() map[string]string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return map[string]string{
			"ru": "app/bot/locales/ru.json",
			"en": "app/bot/locales/en.json",
			"uz": "app/bot/locales/uz.json",
		}
	}

	localesDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "locales"))
	return map[string]string{
		"ru": filepath.Join(localesDir, "ru.json"),
		"en": filepath.Join(localesDir, "en.json"),
		"uz": filepath.Join(localesDir, "uz.json"),
	}
}

package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetAndChangeLanguage(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "ru.json"), `{
		"menu": {
			"hello": "Привет, {name}!",
			"only_ru": "Только русский"
		}
	}`)
	writeTestFile(t, filepath.Join(dir, "en.json"), `{
		"menu": {
			"hello": "Hello, {name}!"
		}
	}`)
	writeTestFile(t, filepath.Join(dir, "uz.json"), `{
		"menu": {
			"hello": "Salom, {name}!"
		}
	}`)

	err := Load(map[string]string{
		"ru": filepath.Join(dir, "ru.json"),
		"en": filepath.Join(dir, "en.json"),
		"uz": filepath.Join(dir, "uz.json"),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := Language(); got != "ru" {
		t.Fatalf("Language() = %q, want %q", got, "ru")
	}

	if got := Get("menu.hello", "name", "Mirfayz"); got != "Привет, Mirfayz!" {
		t.Fatalf("Get() = %q", got)
	}

	if got := ChangeLanguage("en-US"); got != "en" {
		t.Fatalf("ChangeLanguage() = %q, want %q", got, "en")
	}

	if got := Get("menu.hello", map[string]any{"name": "John"}); got != "Hello, John!" {
		t.Fatalf("Get() = %q", got)
	}

	if got := Get("menu.only_ru"); got != "Только русский" {
		t.Fatalf("fallback Get() = %q", got)
	}

	if got := ChangeLanguage("de"); got != "ru" {
		t.Fatalf("ChangeLanguage() fallback = %q, want %q", got, "ru")
	}

	if got := Get("missing.key"); got != "missing.key" {
		t.Fatalf("missing key Get() = %q", got)
	}
}

func TestLocalizerFluentAPI(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "ru.json"), `{
		"menu": {
			"buy": "Купить от {MIN} до {MAX} для @{username}"
		}
	}`)
	writeTestFile(t, filepath.Join(dir, "en.json"), `{
		"menu": {
			"buy": "Buy from {MIN} to {MAX} for @{username}"
		}
	}`)

	err := Load(map[string]string{
		"ru": filepath.Join(dir, "ru.json"),
		"en": filepath.Join(dir, "en.json"),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	text := For("en-US").Get("menu.buy")
	if got := text.Raw(); got != "Buy from {MIN} to {MAX} for @{username}" {
		t.Fatalf("Raw() = %q", got)
	}

	if got := text.Format("MIN", 50, "MAX", 1000, "username", "john"); got != "Buy from 50 to 1000 for @john" {
		t.Fatalf("Format() = %q", got)
	}

	if got := For("de").Language(); got != "ru" {
		t.Fatalf("Localizer.Language() = %q, want %q", got, "ru")
	}

	if got := LookupFor("de", "missing.key").Format("username", "john"); got != "missing.key" {
		t.Fatalf("missing Message.Format() = %q", got)
	}
}

func TestUserLanguageOverride(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "ru.json"), `{
		"menu": {
			"hello": "Привет"
		}
	}`)
	writeTestFile(t, filepath.Join(dir, "en.json"), `{
		"menu": {
			"hello": "Hello"
		}
	}`)
	writeTestFile(t, filepath.Join(dir, "uz.json"), `{
		"menu": {
			"hello": "Salom"
		}
	}`)

	err := Load(map[string]string{
		"ru": filepath.Join(dir, "ru.json"),
		"en": filepath.Join(dir, "en.json"),
		"uz": filepath.Join(dir, "uz.json"),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	const userID int64 = 42
	ResetUserLanguage(userID)
	t.Cleanup(func() {
		ResetUserLanguage(userID)
	})

	if got := ForUser(userID).Get("menu.hello").String(); got != "Привет" {
		t.Fatalf("ForUser default = %q", got)
	}

	if got := SetUserLanguage(userID, "uz-UZ"); got != "uz" {
		t.Fatalf("SetUserLanguage() = %q, want %q", got, "uz")
	}

	if got := UserLanguage(userID); got != "uz" {
		t.Fatalf("UserLanguage() = %q, want %q", got, "uz")
	}

	if got := ForUser(userID).Get("menu.hello").String(); got != "Salom" {
		t.Fatalf("ForUser override = %q", got)
	}
}

func TestMainAliases(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "ru.json"), `{
		"menu": {
			"start": "Старт",
			"buy_list": {
				"stars": {
					"username": "Введите username",
					"self": "Купить {amount} звезд"
				}
			}
		}
	}`)

	err := Load(map[string]string{
		"ru": filepath.Join(dir, "ru.json"),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tr := For("ru")

	if got := tr.Main("start").String(); got != "Старт" {
		t.Fatalf("Main(start) = %q", got)
	}

	if got := tr.Get("main.stars.username").String(); got != "Введите username" {
		t.Fatalf("main.stars.username = %q", got)
	}

	if got := tr.Main("stars.self").Format("amount", 50); got != "Купить 50 звезд" {
		t.Fatalf("Main(stars.self).Format() = %q", got)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

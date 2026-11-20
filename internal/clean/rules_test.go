package clean

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestChromeKeepsProfileData(t *testing.T) {
	c := testContext(t)
	cache := filepath.Join(c.Env.CacheDir, "google-chrome", "Default")
	conf := filepath.Join(c.Env.ConfigDir, "google-chrome")
	write(t, filepath.Join(cache, "Cache", "Cache_Data", "f_0001"), 9000)
	write(t, filepath.Join(cache, "Code Cache", "js", "a"), 9000)
	write(t, filepath.Join(conf, "Default", "GPUCache", "data_0"), 9000)
	write(t, filepath.Join(conf, "ShaderCache", "x"), 9000)
	keep := []string{
		filepath.Join(conf, "Default", "Cookies"),
		filepath.Join(conf, "Default", "History"),
		filepath.Join(conf, "Default", "Login Data"),
		filepath.Join(conf, "Default", "Bookmarks"),
	}
	for _, k := range keep {
		write(t, k, 100)
	}

	f := findingFor(Scan(context.Background(), c, userRules(), nil), "chrome")
	if f == nil || len(f.Targets) != 4 {
		t.Fatalf("chrome finding: %+v", f)
	}
	Clean(context.Background(), c, *f)
	for _, k := range keep {
		if !exists(k) {
			t.Errorf("%s was removed", k)
		}
	}
	if exists(filepath.Join(cache, "Cache")) {
		t.Error("cache was not removed")
	}
}

func TestFirefoxSnapAndDeb(t *testing.T) {
	c := testContext(t)
	deb := filepath.Join(c.Env.CacheDir, "mozilla", "firefox", "abc.default", "cache2")
	snap := filepath.Join(c.Env.Home, "snap", "firefox", "common", ".cache", "mozilla", "firefox", "xyz.default", "cache2")
	write(t, filepath.Join(deb, "entries", "1"), 5000)
	write(t, filepath.Join(snap, "entries", "1"), 5000)
	profile := filepath.Join(c.Env.Home, ".mozilla", "firefox", "abc.default", "places.sqlite")
	write(t, profile, 100)

	found := Scan(context.Background(), c, userRules(), nil)
	f := findingFor(found, "firefox")
	if f == nil || !slices.Equal(paths(f.Targets), []string{deb, snap}) {
		t.Fatalf("firefox finding: %+v", f)
	}
	if s := findingFor(found, "snap-caches"); s != nil {
		t.Errorf("snap-caches should not claim the Firefox snap cache: %v", paths(s.Targets))
	}
	Clean(context.Background(), c, *f)
	if !exists(profile) {
		t.Error("Firefox profile was touched")
	}
}

func TestVSCodeKeepsSettings(t *testing.T) {
	c := testContext(t)
	code := filepath.Join(c.Env.ConfigDir, "Code")
	write(t, filepath.Join(code, "CachedData", "abc", "x"), 4000)
	write(t, filepath.Join(code, "logs", "2026", "main.log"), 4000)
	settings := filepath.Join(code, "User", "settings.json")
	write(t, settings, 100)

	f := findingFor(Scan(context.Background(), c, userRules(), nil), "vscode")
	if f == nil || len(f.Targets) != 2 {
		t.Fatalf("vscode finding: %+v", f)
	}
	Clean(context.Background(), c, *f)
	if !exists(settings) {
		t.Error("settings.json was removed")
	}
}

func TestPartialCommandShowsUpTo(t *testing.T) {
	c := testContext(t)
	write(t, filepath.Join(c.Env.DataDir, "pnpm", "store", "v3", "files", "a"), 9000)
	c.LookPath = func(string) (string, error) { return "/usr/bin/pnpm", nil }
	f := findingFor(Scan(context.Background(), c, Available(c, userRules()), nil), "pnpm")
	if f == nil || !f.Unknown || f.Bytes <= 0 {
		t.Fatalf("pnpm should report an upper bound: %+v", f)
	}
}

func TestGoModCacheLookup(t *testing.T) {
	c := testContext(t)
	t.Setenv("GOMODCACHE", "")
	t.Setenv("GOPATH", "")
	if got := goModCache(c); got[0] != filepath.Join(c.Env.Home, "go", "pkg", "mod") {
		t.Errorf("default: %v", got)
	}
	t.Setenv("GOPATH", "/opt/gopath:/other")
	if got := goModCache(c); got[0] != "/opt/gopath/pkg/mod" {
		t.Errorf("GOPATH: %v", got)
	}
	t.Setenv("GOMODCACHE", "/fast/mod")
	if got := goModCache(c); got[0] != "/fast/mod" {
		t.Errorf("GOMODCACHE: %v", got)
	}
}

func TestDockerBuildCache(t *testing.T) {
	c := testContext(t)
	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Images\t2.1GB (40%)\nContainers\t0B (0%)\nLocal Volumes\t1GB (100%)\nBuild Cache\t1.234GB\n"), nil
	}
	ts, err := dockerBuildCache(context.Background(), c)
	if err != nil || len(ts) != 1 || ts[0].Bytes != 1_234_000_000 {
		t.Errorf("got %+v, %v", ts, err)
	}
	c.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, os.ErrPermission
	}
	if ts, err := dockerBuildCache(context.Background(), c); ts != nil || err != nil {
		t.Errorf("unreachable daemon should find nothing: %+v %v", ts, err)
	}
}

func TestParseUnitSize(t *testing.T) {
	cases := map[string]int64{
		"0B": 0, "300kB": 300_000, "512MB (45%)": 512_000_000, "1.5G": 1_500_000_000,
		"2TB": 2_000_000_000_000, "2KiB": 2048, "1MiB": 1024 * 1024, "1GiB": 1024 * 1024 * 1024,
		"42": 42, "": 0, "abc": 0, "7XB": 0,
	}
	for in, want := range cases {
		if got := parseUnitSize(in); got != want {
			t.Errorf("parseUnitSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestTelegramKeepsLoginData(t *testing.T) {
	c := testContext(t)
	tdata := filepath.Join(c.Env.DataDir, "TelegramDesktop", "tdata")
	cacheFile := filepath.Join(tdata, "user_data", "cache", "img.bin")
	keyFile := filepath.Join(tdata, "key_datas")
	write(t, cacheFile, 5000)
	write(t, keyFile, 100)

	f := findingFor(Scan(context.Background(), c, userRules(), nil), "telegram")
	if f == nil || len(f.Targets) != 1 {
		t.Fatalf("telegram finding: %+v", f)
	}
	Clean(context.Background(), c, *f)
	if exists(cacheFile) {
		t.Error("telegram cache should be removed")
	}
	if !exists(keyFile) {
		t.Error("telegram login data was removed")
	}
}

func TestSteamAndAndroidKeepUserData(t *testing.T) {
	c := testContext(t)
	steam := filepath.Join(c.Env.DataDir, "Steam")
	write(t, filepath.Join(steam, "appcache", "httpcache", "a"), 4000)
	game := filepath.Join(steam, "steamapps", "common", "Game", "save.dat")
	write(t, game, 100)

	android := filepath.Join(c.Env.Home, ".android")
	write(t, filepath.Join(android, "build-cache", "obj"), 4000)
	keystore := filepath.Join(android, "debug.keystore")
	write(t, keystore, 100)

	found := Scan(context.Background(), c, userRules(), nil)
	sf := findingFor(found, "steam-cache")
	af := findingFor(found, "android-cache")
	if sf == nil || af == nil {
		t.Fatalf("expected steam-cache and android-cache findings: steam=%+v android=%+v", sf, af)
	}
	Clean(context.Background(), c, *sf)
	Clean(context.Background(), c, *af)
	if !exists(game) {
		t.Error("steam game file was removed")
	}
	if !exists(keystore) {
		t.Error("android keystore was removed")
	}
}

func TestUvEnvOverrideAndZenBrowser(t *testing.T) {
	c := testContext(t)
	customUV := filepath.Join(c.Env.Home, "fast-uv")
	t.Setenv("UV_CACHE_DIR", customUV)
	write(t, filepath.Join(customUV, "archive-v0", "pkg.whl"), 6000)
	write(t, filepath.Join(c.Env.CacheDir, "zen", "abc.default", "cache2", "entries", "1"), 4000)

	found := Scan(context.Background(), c, userRules(), nil)
	if f := findingFor(found, "uv"); f == nil || len(f.Targets) != 1 {
		t.Fatalf("uv finding with UV_CACHE_DIR: %+v", f)
	}
	if f := findingFor(found, "zen"); f == nil || len(f.Targets) != 1 {
		t.Fatalf("zen finding: %+v", f)
	}
}

func TestRulesHaveSafeDescriptions(t *testing.T) {
	for _, r := range Rules() {
		i := r.Info()
		if strings.HasSuffix(i.Description, " ") || !strings.HasSuffix(i.Description, ".") {
			t.Errorf("rule %q description should be a full sentence: %q", i.ID, i.Description)
		}
	}
}

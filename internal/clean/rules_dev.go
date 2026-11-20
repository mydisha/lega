package clean

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mydisha/lega/internal/scan"
)

// electronCache lists the cache folders inside an Electron app's config
// folder (VS Code, Slack, Discord, ...).
var electronCache = []string{"Cache", "CachedData", "CachedExtensionVSIXs", "Code Cache", "GPUCache", "Service Worker/CacheStorage", "logs"}

func cacheRule(id, name, group, desc string, risk Risk, root string, globs ...string) Rule {
	return &pathRule{
		info:  Info{ID: id, Name: name, Group: group, Description: desc, Risk: risk},
		spots: []spot{{root, globs}},
	}
}

func devRules() []Rule {
	const rebuilt = "Downloaded packages. They are fetched again when a project needs them."
	return []Rule{
		cacheRule("npm", "npm cache", GroupDev, rebuilt, Safe, "$ENV:NPM_CONFIG_CACHE|~/.npm", "_cacache", "_logs", "_npx"),
		&pathRule{
			info: Info{ID: "yarn", Name: "Yarn cache", Group: GroupDev, Description: rebuilt},
			spots: []spot{
				{"$ENV:YARN_CACHE_FOLDER|$CACHE/yarn", []string{"*"}},
				{"~/.yarn/berry/cache", []string{"*"}},
				{"$CACHE/node/corepack", []string{"*"}},
			},
		},
		&commandRule{
			info: Info{
				ID: "pnpm", Name: "pnpm store", Group: GroupDev, Needs: []string{"pnpm"},
				Description: "Runs \"pnpm store prune\", which removes packages no project uses.",
			},
			measure: []string{"$DATA/pnpm/store", "$CACHE/pnpm/store"},
			cmd:     []string{"pnpm", "store", "prune"},
			partial: true,
		},
		cacheRule("pip", "pip cache", GroupDev, rebuilt, Safe, "$ENV:PIP_CACHE_DIR|$CACHE/pip", "*"),
		cacheRule("uv", "uv cache", GroupDev, rebuilt, Safe, "$ENV:UV_CACHE_DIR|$CACHE/uv", "*"),
		cacheRule("poetry", "Poetry cache", GroupDev, rebuilt, Safe, "$CACHE/pypoetry", "cache/*", "artifacts/*"),
		cacheRule("pre-commit", "pre-commit cache", GroupDev,
			"Cached git hooks and environments for pre-commit. They are rebuilt on the next commit.", Review,
			"$CACHE/pre-commit", "*"),
		&commandRule{
			info: Info{
				ID: "conda", Name: "Conda package cache", Group: GroupDev, Risk: Review, Needs: []string{"conda"},
				Description: "Runs \"conda clean --tarballs --packages --index-cache --yes\". Active environments stay intact.",
			},
			measureFn: measurePaths(condaPkgDirs),
			cmd:       []string{"conda", "clean", "--tarballs", "--packages", "--index-cache", "--yes"},
			partial:   true,
		},
		&pathRule{
			info: Info{
				ID: "go-build", Name: "Go build cache", Group: GroupDev, Processes: []string{"go"},
				Description: "Compiled Go packages. Builds are slower once while it fills again.",
			},
			spots: []spot{{"$ENV:GOCACHE|$CACHE/go-build", []string{"*"}}},
		},
		&commandRule{
			info: Info{
				ID: "go-mod", Name: "Go module cache", Group: GroupDev, Risk: Review, Needs: []string{"go"},
				Description: "Downloaded Go modules, removed with \"go clean -modcache\". Offline builds fail until they are fetched again.",
			},
			measureFn: measurePaths(goModCache),
			cmd:       []string{"go", "clean", "-modcache"},
		},
		cacheRule("cargo", "Cargo registry cache", GroupDev, rebuilt, Review, "$ENV:CARGO_HOME|~/.cargo", "registry/cache/*", "registry/src/*", "git/checkouts/*"),
		cacheRule("composer", "Composer cache", GroupDev, rebuilt, Safe, "$ENV:COMPOSER_CACHE_DIR|$CACHE/composer", "*"),
		cacheRule("bun", "Bun cache", GroupDev, rebuilt, Safe, "$ENV:BUN_INSTALL_CACHE_DIR|~/.bun/install/cache", "*"),
		cacheRule("deno", "Deno cache", GroupDev, rebuilt, Safe, "$ENV:DENO_DIR|$CACHE/deno", "*"),
		cacheRule("gradle", "Gradle caches", GroupDev, rebuilt, Review, "$ENV:GRADLE_USER_HOME|~/.gradle", "caches/*", "daemon/*", "wrapper/dists/*"),
		cacheRule("maven", "Maven repository", GroupDev, rebuilt, Review, "~/.m2/repository", "*"),
		cacheRule("pub-cache", "Dart and Flutter pub cache", GroupDev, rebuilt, Review, "$ENV:PUB_CACHE|~/.pub-cache", "*"),
		cacheRule("android-cache", "Android build cache", GroupDev,
			"Build and download caches in ~/.android. Emulators and signing keys are kept.", Safe,
			"~/.android", "build-cache/*", "cache/*"),
		&pathRule{
			info: Info{
				ID: "compiler-cache", Name: "ccache and sccache", Group: GroupDev,
				Description: "Cached C, C++ and Rust object files. Builds are slower once while they refill.",
			},
			spots: []spot{
				{"$ENV:CCACHE_DIR|$CACHE/ccache", []string{"*"}},
				{"$ENV:SCCACHE_DIR|$CACHE/sccache", []string{"*"}},
			},
		},
		&pathRule{
			info: Info{
				ID: "nuget", Name: "NuGet package cache", Group: GroupDev, Risk: Review,
				Description: rebuilt,
			},
			spots: []spot{
				{"~/.nuget/packages", []string{"*"}},
				{"$DATA/NuGet", []string{"v3-cache/*", "http-cache/*"}},
			},
		},
		&pathRule{
			info: Info{ID: "bundler", Name: "Ruby Bundler cache", Group: GroupDev, Description: rebuilt},
			spots: []spot{
				{"~/.bundle/cache", []string{"*"}},
				{"$CACHE/bundler", []string{"*"}},
			},
		},
		cacheRule("homebrew", "Homebrew download cache", GroupDev,
			"Downloaded package archives kept by Homebrew. Installed tools stay intact.", Safe,
			"$CACHE/Homebrew", "*"),
		&pathRule{
			info: Info{
				ID: "playwright", Name: "Browser test runtimes", Group: GroupDev, Risk: Review,
				Description: "Downloaded browsers and binaries for Playwright, Puppeteer, Cypress and Electron. Re-downloading can take a while.",
			},
			spots: []spot{
				{"$CACHE/ms-playwright", []string{"*"}},
				{"$CACHE/puppeteer", []string{"*"}},
				{"$CACHE/Cypress", []string{"*"}},
				{"$CACHE/electron", []string{"*"}},
				{"$CACHE/electron-builder", []string{"*"}},
			},
		},
		cacheRule("jetbrains", "JetBrains IDE caches", GroupDev,
			"Indexes and logs of JetBrains IDEs. The IDE re-indexes projects on the next start.", Review,
			"$CACHE/JetBrains", "*"),
		&pathRule{
			info: Info{
				ID: "vscode", Name: "VS Code caches", Group: GroupDev,
				Processes:   []string{"code", "code-insiders", "codium", "cursor", "windsurf"},
				Description: "Caches and logs of VS Code, VSCodium, Cursor and Windsurf. Settings and extensions are kept.",
			},
			spots: []spot{
				{"$CONFIG/Code", electronCache},
				{"$CONFIG/Code - Insiders", electronCache},
				{"$CONFIG/VSCodium", electronCache},
				{"$CONFIG/Cursor", electronCache},
				{"$CONFIG/Windsurf", electronCache},
			},
		},
		&pathRule{
			info: Info{
				ID: "zed", Name: "Zed editor cache", Group: GroupDev,
				Processes:   []string{"zed", "zed-editor"},
				Description: "Caches of the Zed editor. Settings and extensions are kept.",
			},
			spots: []spot{{"$CACHE/zed", []string{"*"}}},
		},
		cacheRule("huggingface", "Hugging Face model cache", GroupDev,
			"Downloaded Hugging Face models and datasets. Large models will need to be downloaded again.", Review,
			"$ENV:HF_HOME|$CACHE/huggingface", "hub/*"),
		&pathRule{
			info: Info{
				ID: "torch-cache", Name: "PyTorch and Whisper caches", Group: GroupDev, Risk: Review,
				Description: "Downloaded ML model weights in ~/.cache/torch and ~/.cache/whisper. They are downloaded again when needed.",
			},
			spots: []spot{
				{"$CACHE/torch", []string{"*"}},
				{"$CACHE/whisper", []string{"*"}},
			},
		},
		&commandRule{
			info: Info{
				ID: "docker-build", Name: "Docker build cache", Group: GroupDev, Risk: Review, Needs: []string{"docker"},
				Description: "Runs \"docker builder prune -f\". Images and containers are kept.",
			},
			measureFn: dockerBuildCache,
			cmd:       []string{"docker", "builder", "prune", "-f"},
		},
		&commandRule{
			info: Info{
				ID: "podman-build", Name: "Podman unused images", Group: GroupDev, Risk: Review, Needs: []string{"podman"},
				Description: "Runs \"podman image prune -f\" to remove dangling images. Containers and tagged images are kept.",
			},
			measureFn: podmanImageCache,
			cmd:       []string{"podman", "image", "prune", "-f"},
			partial:   true,
		},
	}
}

func appRules() []Rule {
	return []Rule{
		&pathRule{
			info: Info{
				ID: "chat-apps", Name: "Chat app caches", Group: GroupApps,
				Processes:   []string{"slack", "Discord", "discord", "teams", "teams-for-linux", "vesktop", "element-desktop"},
				Description: "Caches of Slack, Discord, Teams, Vesktop and Element. Messages stay on their servers.",
			},
			spots: []spot{
				{"$CONFIG/Slack", electronCache},
				{"~/snap/slack/common/.config/Slack", electronCache},
				{"~/.var/app/com.slack.Slack/config/Slack", electronCache},
				{"$CONFIG/discord", electronCache},
				{"~/snap/discord/common/.config/discord", electronCache},
				{"~/.var/app/com.discordapp.Discord/config/discord", electronCache},
				{"$CONFIG/vesktop", electronCache},
				{"~/.var/app/dev.vencord.Vesktop/config/vesktop", electronCache},
				{"$CONFIG/Element", electronCache},
				{"~/.var/app/im.riot.Riot/config/Element", electronCache},
				{"$CONFIG/Microsoft/Microsoft Teams", electronCache},
				{"$CONFIG/teams-for-linux", electronCache},
			},
		},
		&pathRule{
			info: Info{
				ID: "telegram", Name: "Telegram Desktop cache", Group: GroupApps,
				Processes:   []string{"telegram-desktop", "Telegram"},
				Description: "Cached media of Telegram Desktop. Chats and logins stay intact.",
			},
			spots: []spot{
				{"$DATA/TelegramDesktop/tdata/user_data", []string{"cache/*", "media_cache/*"}},
				{"~/snap/telegram-desktop/common/.local/share/TelegramDesktop/tdata/user_data", []string{"cache/*", "media_cache/*"}},
				{"~/.var/app/org.telegram.desktop/data/TelegramDesktop/tdata/user_data", []string{"cache/*", "media_cache/*"}},
			},
		},
		&pathRule{
			info: Info{
				ID: "spotify", Name: "Spotify cache", Group: GroupApps,
				Processes:   []string{"spotify"},
				Description: "Cached songs and album art of Spotify. Playlists and logins are kept.",
			},
			spots: []spot{
				{"$CACHE/spotify", []string{"*"}},
				{"~/snap/spotify/common/.cache/spotify", []string{"*"}},
				{"~/.var/app/com.spotify.Client/cache/spotify", []string{"*"}},
			},
		},
		&pathRule{
			info: Info{
				ID: "steam-cache", Name: "Steam cache and logs", Group: GroupApps,
				Processes:   []string{"steam", "steamwebhelper"},
				Description: "Steam browser cache, download cache and logs. Installed games and saves are kept.",
			},
			spots: []spot{
				{"$DATA/Steam", []string{"appcache/httpcache/*", "depotcache/*", "logs/*"}},
				{"~/.var/app/com.valvesoftware.Steam/.local/share/Steam", []string{"appcache/httpcache/*", "depotcache/*", "logs/*"}},
			},
		},
		&pathRule{
			info: Info{
				ID: "gpu-shaders", Name: "GPU shader caches", Group: GroupApps,
				Description: "Compiled Mesa, Vulkan and NVIDIA shaders. Graphics drivers rebuild them as needed.",
			},
			spots: []spot{
				{"$CACHE/mesa_shader_cache", []string{"*"}},
				{"$CACHE/mesa_shader_cache_db", []string{"*"}},
				{"$CACHE/nvidia/GLCache", []string{"*"}},
				{"$CACHE", []string{"radv_builtin_shaders*"}},
			},
		},
	}
}

// measurePaths turns a function that finds folders into a measureFn.
func measurePaths(find func(c *Context) []string) func(context.Context, *Context) ([]Target, error) {
	return func(ctx context.Context, c *Context) ([]Target, error) {
		var out []Target
		for _, p := range find(c) {
			bytes, files, err := scan.Size(ctx, p)
			if err == nil && bytes > 0 {
				out = append(out, Target{Path: p, Bytes: bytes, Files: files})
			}
		}
		return out, nil
	}
}

// goModCache follows Go's own lookup: $GOMODCACHE, then $GOPATH/pkg/mod,
// then ~/go/pkg/mod.
func goModCache(c *Context) []string {
	if p := os.Getenv("GOMODCACHE"); filepath.IsAbs(p) {
		return []string{p}
	}
	if gp := filepath.SplitList(os.Getenv("GOPATH")); len(gp) > 0 && filepath.IsAbs(gp[0]) {
		return []string{filepath.Join(gp[0], "pkg", "mod")}
	}
	return []string{filepath.Join(c.Env.Home, "go", "pkg", "mod")}
}

// condaPkgDirs lists standard Conda/Mamba package cache directories.
func condaPkgDirs(c *Context) []string {
	var out []string
	if v := os.Getenv("CONDA_PKGS_DIRS"); v != "" {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if filepath.IsAbs(p) {
				out = append(out, filepath.Clean(p))
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	for _, rel := range []string{"miniconda3/pkgs", "anaconda3/pkgs", "miniforge3/pkgs", "mambaforge/pkgs", ".conda/pkgs"} {
		out = append(out, filepath.Join(c.Env.Home, rel))
	}
	return append(out, filepath.Join(c.Env.CacheDir, "conda", "pkgs"))
}

// dockerBuildCache asks Docker how much build cache it could free. When the
// daemon is not reachable the rule simply finds nothing.
func dockerBuildCache(ctx context.Context, c *Context) ([]Target, error) {
	out, err := c.Run(ctx, "docker", "system", "df", "--format", "{{.Type}}\t{{.Reclaimable}}")
	if err != nil {
		return nil, nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		kind, val, ok := strings.Cut(line, "\t")
		if !ok || kind != "Build Cache" {
			continue
		}
		if n := parseUnitSize(val); n > 0 {
			return []Target{{Path: "docker build cache", Bytes: n}}, nil
		}
	}
	return nil, nil
}

// podmanImageCache asks Podman how much image space can be reclaimed.
func podmanImageCache(ctx context.Context, c *Context) ([]Target, error) {
	out, err := c.Run(ctx, "podman", "system", "df", "--format", "{{.Type}}\t{{.Reclaimable}}")
	if err != nil {
		return nil, nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		kind, val, ok := strings.Cut(line, "\t")
		if !ok || kind != "Images" {
			continue
		}
		if n := parseUnitSize(val); n > 0 {
			return []Target{{Path: "podman unused images", Bytes: n}}, nil
		}
	}
	return nil, nil
}

// parseUnitSize reads sizes such as "1.2GB", "512MB (45%)", "300kB", "0B",
// "1.5G" (as printed by journalctl) and binary units like "2GiB".
func parseUnitSize(s string) int64 {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	mult := map[string]float64{
		"": 1, "B": 1,
		"K": 1e3, "KB": 1e3, "kB": 1e3,
		"M": 1e6, "MB": 1e6,
		"G": 1e9, "GB": 1e9,
		"T": 1e12, "TB": 1e12,
		"Ki": 1024, "KiB": 1024,
		"Mi": 1024 * 1024, "MiB": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024, "GiB": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024, "TiB": 1024 * 1024 * 1024 * 1024,
	}[s[i:]]
	return int64(num * mult)
}

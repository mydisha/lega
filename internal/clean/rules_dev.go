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
		cacheRule("npm", "npm cache", GroupDev, rebuilt, Safe, "~/.npm", "_cacache", "_logs"),
		cacheRule("yarn", "Yarn cache", GroupDev, rebuilt, Safe, "$CACHE/yarn", "*"),
		&commandRule{
			info: Info{
				ID: "pnpm", Name: "pnpm store", Group: GroupDev, Needs: []string{"pnpm"},
				Description: "Runs \"pnpm store prune\", which removes packages no project uses.",
			},
			measure: []string{"$DATA/pnpm/store"},
			cmd:     []string{"pnpm", "store", "prune"},
			partial: true,
		},
		cacheRule("pip", "pip cache", GroupDev, rebuilt, Safe, "$CACHE/pip", "*"),
		&pathRule{
			info: Info{
				ID: "go-build", Name: "Go build cache", Group: GroupDev, Processes: []string{"go"},
				Description: "Compiled Go packages. Builds are slower once while it fills again.",
			},
			spots: []spot{{"$CACHE/go-build", []string{"*"}}},
		},
		&commandRule{
			info: Info{
				ID: "go-mod", Name: "Go module cache", Group: GroupDev, Risk: Review, Needs: []string{"go"},
				Description: "Downloaded Go modules, removed with \"go clean -modcache\". Offline builds fail until they are fetched again.",
			},
			measureFn: measurePaths(goModCache),
			cmd:       []string{"go", "clean", "-modcache"},
		},
		cacheRule("cargo", "Cargo registry cache", GroupDev, rebuilt, Review, "~/.cargo", "registry/cache/*", "registry/src/*", "git/checkouts/*"),
		cacheRule("composer", "Composer cache", GroupDev, rebuilt, Safe, "$CACHE/composer", "*"),
		cacheRule("bun", "Bun cache", GroupDev, rebuilt, Safe, "~/.bun/install/cache", "*"),
		cacheRule("deno", "Deno cache", GroupDev, rebuilt, Safe, "$CACHE/deno", "*"),
		cacheRule("gradle", "Gradle caches", GroupDev, rebuilt, Review, "~/.gradle", "caches/*", "daemon/*", "wrapper/dists/*"),
		cacheRule("maven", "Maven repository", GroupDev, rebuilt, Review, "~/.m2/repository", "*"),
		cacheRule("jetbrains", "JetBrains IDE caches", GroupDev,
			"Indexes and logs of JetBrains IDEs. The IDE re-indexes projects on the next start.", Review,
			"$CACHE/JetBrains", "*"),
		&pathRule{
			info: Info{
				ID: "vscode", Name: "VS Code caches", Group: GroupDev,
				Processes:   []string{"code", "code-insiders", "codium", "cursor"},
				Description: "Caches and logs of VS Code, VSCodium and Cursor. Settings and extensions are kept.",
			},
			spots: []spot{
				{"$CONFIG/Code", electronCache},
				{"$CONFIG/Code - Insiders", electronCache},
				{"$CONFIG/VSCodium", electronCache},
				{"$CONFIG/Cursor", electronCache},
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
	}
}

func appRules() []Rule {
	return []Rule{
		&pathRule{
			info: Info{
				ID: "chat-apps", Name: "Chat app caches", Group: GroupApps,
				Processes:   []string{"slack", "Discord", "discord", "teams", "teams-for-linux"},
				Description: "Caches of Slack, Discord and Teams. Messages stay on their servers.",
			},
			spots: []spot{
				{"$CONFIG/Slack", electronCache},
				{"$CONFIG/discord", electronCache},
				{"$CONFIG/Microsoft/Microsoft Teams", electronCache},
				{"$CONFIG/teams-for-linux", electronCache},
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

// parseUnitSize reads sizes such as "1.2GB", "512MB (45%)", "300kB", "0B"
// and "1.5G" (as printed by journalctl).
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
	}[s[i:]]
	return int64(num * mult)
}

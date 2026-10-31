package clean

// Only cache folders are listed here. Profiles, cookies, history and
// passwords live next to them and are never matched.

var (
	// chromiumCache sits under ~/.cache/<browser>/<profile>.
	chromiumCache = []string{"*/Cache", "*/Code Cache"}
	// chromiumConfigCache sits under ~/.config/<browser>.
	chromiumConfigCache = []string{
		"*/GPUCache", "*/Service Worker/CacheStorage", "*/Service Worker/ScriptCache",
		"*/DawnGraphiteCache", "*/DawnWebGPUCache",
		"ShaderCache", "GrShaderCache", "GraphiteDawnCache",
	}
	firefoxCache = []string{"*/cache2", "*/startupCache", "*/thumbnails"}
)

// chromiumBrowser builds a rule for a Chromium-based browser that stores its
// data in folder (relative to ~/.cache and ~/.config).
func chromiumBrowser(id, name string, procs []string, folders ...string) Rule {
	var spots []spot
	for _, f := range folders {
		spots = append(spots,
			spot{"$CACHE/" + f, chromiumCache},
			spot{"$CONFIG/" + f, chromiumConfigCache},
		)
	}
	return &pathRule{
		info: Info{
			ID: id, Name: name, Group: GroupBrowsers, Processes: procs,
			Description: "Web page cache. Logins, history and bookmarks are kept.",
		},
		spots: spots,
	}
}

func browserRules() []Rule {
	chromium := chromiumBrowser("chromium", "Chromium cache", []string{"chromium", "chromium-browser", "chrome"}, "chromium").(*pathRule)
	// The Chromium snap keeps its data inside ~/snap.
	chromium.spots = append(chromium.spots,
		spot{"~/snap/chromium/common/.cache/chromium", chromiumCache},
		spot{"~/snap/chromium/common/chromium", chromiumConfigCache},
	)

	return []Rule{
		&pathRule{
			info: Info{
				ID: "firefox", Name: "Firefox cache", Group: GroupBrowsers,
				Processes:   []string{"firefox", "firefox-bin", "firefox-esr"},
				Description: "Web page cache. Logins, history and bookmarks are kept.",
			},
			spots: []spot{
				{"$CACHE/mozilla/firefox", firefoxCache},
				{"~/snap/firefox/common/.cache/mozilla/firefox", firefoxCache},
				{"~/.var/app/org.mozilla.firefox/cache/mozilla/firefox", firefoxCache},
			},
		},
		chromiumBrowser("chrome", "Google Chrome cache", []string{"chrome", "google-chrome"}, "google-chrome", "google-chrome-beta"),
		chromium,
		chromiumBrowser("brave", "Brave cache", []string{"brave", "brave-browser"}, "BraveSoftware/Brave-Browser"),
		chromiumBrowser("edge", "Microsoft Edge cache", []string{"msedge", "microsoft-edge"}, "microsoft-edge"),
		chromiumBrowser("vivaldi", "Vivaldi cache", []string{"vivaldi", "vivaldi-bin"}, "vivaldi"),
		chromiumBrowser("opera", "Opera cache", []string{"opera"}, "opera"),
	}
}

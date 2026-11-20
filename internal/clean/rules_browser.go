package clean

// Only cache folders are listed here. Profiles, cookies, history and
// passwords live next to them and are never matched.

var (
	// chromiumCache sits under ~/.cache/<browser>/<profile>.
	chromiumCache = []string{"*/Cache", "*/Code Cache", "*/Media Cache"}
	// chromiumConfigCache sits under ~/.config/<browser>.
	chromiumConfigCache = []string{
		"*/GPUCache", "*/Service Worker/CacheStorage", "*/Service Worker/ScriptCache",
		"*/DawnCache", "*/DawnGraphiteCache", "*/DawnWebGPUCache",
		"ShaderCache", "GrShaderCache", "GraphiteDawnCache", "BrowserMetrics",
	}
	firefoxCache = []string{"*/cache2", "*/startupCache", "*/thumbnails"}
)

// chromiumBrowser builds a rule for a Chromium-based browser that stores its
// data in folder (relative to ~/.cache and ~/.config).
func chromiumBrowser(id, name string, procs []string, folders ...string) *pathRule {
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

func flatpakChromiumSpots(appID, folder string) []spot {
	base := "~/.var/app/" + appID
	return []spot{
		{base + "/cache/" + folder, chromiumCache},
		{base + "/config/" + folder, chromiumConfigCache},
	}
}

func browserRules() []Rule {
	chrome := chromiumBrowser("chrome", "Google Chrome cache", []string{"chrome", "google-chrome"}, "google-chrome", "google-chrome-beta", "google-chrome-unstable")
	chrome.spots = append(chrome.spots, flatpakChromiumSpots("com.google.Chrome", "google-chrome")...)

	chromium := chromiumBrowser("chromium", "Chromium cache", []string{"chromium", "chromium-browser", "chrome"}, "chromium")
	chromium.spots = append(chromium.spots,
		spot{"~/snap/chromium/common/.cache/chromium", chromiumCache},
		spot{"~/snap/chromium/common/chromium", chromiumConfigCache},
	)
	chromium.spots = append(chromium.spots, flatpakChromiumSpots("org.chromium.Chromium", "chromium")...)

	brave := chromiumBrowser("brave", "Brave cache", []string{"brave", "brave-browser"}, "BraveSoftware/Brave-Browser")
	brave.spots = append(brave.spots,
		spot{"~/snap/brave/common/.cache/BraveSoftware/Brave-Browser", chromiumCache},
		spot{"~/snap/brave/common/.config/BraveSoftware/Brave-Browser", chromiumConfigCache},
	)
	brave.spots = append(brave.spots, flatpakChromiumSpots("com.brave.Browser", "BraveSoftware/Brave-Browser")...)

	edge := chromiumBrowser("edge", "Microsoft Edge cache", []string{"msedge", "microsoft-edge"}, "microsoft-edge")
	edge.spots = append(edge.spots, flatpakChromiumSpots("com.microsoft.Edge", "microsoft-edge")...)

	vivaldi := chromiumBrowser("vivaldi", "Vivaldi cache", []string{"vivaldi", "vivaldi-bin"}, "vivaldi")
	vivaldi.spots = append(vivaldi.spots,
		spot{"~/snap/vivaldi/common/.cache/vivaldi", chromiumCache},
		spot{"~/snap/vivaldi/common/.config/vivaldi", chromiumConfigCache},
	)
	vivaldi.spots = append(vivaldi.spots, flatpakChromiumSpots("com.vivaldi.Vivaldi", "vivaldi")...)

	opera := chromiumBrowser("opera", "Opera cache", []string{"opera"}, "opera")
	opera.spots = append(opera.spots,
		spot{"~/snap/opera/common/.cache/opera", chromiumCache},
		spot{"~/snap/opera/common/.config/opera", chromiumConfigCache},
	)
	opera.spots = append(opera.spots, flatpakChromiumSpots("com.opera.Opera", "opera")...)

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
		&pathRule{
			info: Info{
				ID: "zen", Name: "Zen Browser cache", Group: GroupBrowsers,
				Processes:   []string{"zen", "zen-bin"},
				Description: "Web page cache. Logins, history and bookmarks are kept.",
			},
			spots: []spot{
				{"$CACHE/zen", firefoxCache},
				{"~/.var/app/app.zen_browser.zen/cache/zen", firefoxCache},
			},
		},
		&pathRule{
			info: Info{
				ID: "librewolf", Name: "LibreWolf cache", Group: GroupBrowsers,
				Processes:   []string{"librewolf", "librewolf-bin"},
				Description: "Web page cache. Logins, history and bookmarks are kept.",
			},
			spots: []spot{
				{"$CACHE/librewolf", firefoxCache},
				{"~/.var/app/io.gitlab.librewolf-community/cache/librewolf", firefoxCache},
			},
		},
		&pathRule{
			info: Info{
				ID: "epiphany", Name: "GNOME Web cache", Group: GroupBrowsers,
				Processes:   []string{"epiphany"},
				Description: "Web page cache of GNOME Web (Epiphany). Logins, history and bookmarks are kept.",
			},
			spots: []spot{
				{"$CACHE/epiphany", []string{"*"}},
				{"~/.var/app/org.gnome.Epiphany/cache/epiphany", []string{"*"}},
			},
		},
		chrome,
		chromium,
		brave,
		edge,
		vivaldi,
		opera,
	}
}

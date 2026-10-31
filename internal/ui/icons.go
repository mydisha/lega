package ui

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// IconSet picks the icons drawn next to files, folders and apps.
type IconSet int

const (
	// IconsOff draws no icons.
	IconsOff IconSet = iota
	// IconsNerd draws Nerd Font glyphs (https://www.nerdfonts.com). They
	// need a Nerd Font, or a terminal that ships the symbols (Ghostty,
	// WezTerm); otherwise they show as empty boxes.
	IconsNerd
	// IconsEmoji draws emoji, for terminals without a Nerd Font.
	IconsEmoji
)

// glyph is one icon in both sets. Nerd glyphs are one cell wide; emoji are
// two. Emoji are picked from those that show as emoji without a variation
// selector, so every terminal agrees on their width.
type glyph struct{ nerd, emoji string }

// Nerd names come from the Nerd Fonts glyphnames.json.
var (
	icoFolder   = glyph{"\uf07b", "📁"}     // fa-folder
	icoFile     = glyph{"\uf15b", "📄"}     // fa-file
	icoImage    = glyph{"\uf1c5", "📷"}     // fa-file_image_o
	icoVideo    = glyph{"\uf1c8", "🎬"}     // fa-file_video_o
	icoAudio    = glyph{"\uf1c7", "🎵"}     // fa-file_audio_o
	icoArchive  = glyph{"\uf1c6", "📦"}     // fa-file_archive_o
	icoPDF      = glyph{"\uf1c1", "📕"}     // fa-file_pdf_o
	icoDoc      = glyph{"\uf1c2", "📝"}     // fa-file_word_o
	icoSheet    = glyph{"\uf1c3", "📈"}     // fa-file_excel_o
	icoSlides   = glyph{"\uf1c4", "📊"}     // fa-file_powerpoint_o
	icoCode     = glyph{"\uf1c9", "📜"}     // fa-file_code_o
	icoText     = glyph{"\uf15c", "📃"}     // fa-file_text
	icoDatabase = glyph{"\uf1c0", "💾"}     // fa-database
	icoDisk     = glyph{"\uf0a0", "💿"}     // fa-hdd_o
	icoPackage  = glyph{"\uf487", "📦"}     // oct-package
	icoDesktop  = glyph{"\uf108", "💻"}     // fa-desktop
	icoDocs     = glyph{"\uf0f6", "📚"}     // fa-file_text_o
	icoDownload = glyph{"\uf019", "📥"}     // fa-download
	icoMusic    = glyph{"\uf001", "🎵"}     // fa-music
	icoPictures = glyph{"\uf03e", "📷"}     // fa-picture_o
	icoFilm     = glyph{"\uf008", "🎬"}     // fa-film
	icoConfig   = glyph{"\uf013", "🔧"}     // fa-cog
	icoTrash    = glyph{"\uf1f8", "🚮"}     // fa-trash
	icoBug      = glyph{"\uf188", "🐛"}     // fa-bug
	icoChat     = glyph{"\uf086", "💬"}     // fa-comments
	icoApp      = glyph{"\uf1b2", "🧩"}     // fa-cube
	icoTux      = glyph{"\uf17c", "🐧"}     // fa-linux
	icoGlobe    = glyph{"\uf0ac", "🌐"}     // fa-globe
	icoBrave    = glyph{"\uf0ac", "🦁"}     // fa-globe; Nerd Fonts has no Brave logo
	icoFirefox  = glyph{"\uf269", "🦊"}     // fa-firefox
	icoChrome   = glyph{"\uf268", "🌐"}     // fa-chrome
	icoOpera    = glyph{"\uf26a", "🌐"}     // fa-opera
	icoEdge     = glyph{"\uf282", "🌐"}     // fa-edge
	icoGit      = glyph{"\uf1d3", "🌿"}     // fa-git
	icoDocker   = glyph{"\uf308", "🐳"}     // linux-docker
	icoSnap     = glyph{"\uf32c", "📦"}     // linux-snappy
	icoFlatpak  = glyph{"\uf324", "📦"}     // linux-flathub
	icoVSCode   = glyph{"\U000f0a1e", "🧰"} // md-microsoft_visual_studio_code
	icoJetBrain = glyph{"\ue808", "🧰"}     // dev-jetbrains
	icoNode     = glyph{"\ue718", "📦"}     // dev-nodejs_small
	icoNpm      = glyph{"\ue71e", "📦"}     // dev-npm
	icoYarn     = glyph{"\ue6a7", "🧶"}     // seti-yarn
	icoBun      = glyph{"\ue76f", "🥟"}     // dev-bun
	icoDeno     = glyph{"\ue7c0", "🦕"}     // dev-denojs
	icoPython   = glyph{"\ue73c", "🐍"}     // dev-python
	icoGo       = glyph{"\ue627", "🐹"}     // seti-go
	icoRust     = glyph{"\ue7a8", "🦀"}     // dev-rust
	icoGradle   = glyph{"\ue7f2", "🐘"}     // dev-gradle
	icoMaven    = glyph{"\ue82c", "☕"}     // dev-maven
	icoComposer = glyph{"\ue783", "🎼"}     // dev-composer
	icoDart     = glyph{"\ue798", "🎯"}     // dev-dart
)

// ruleIcons maps clean rule IDs to the app or kind of data they clean.
var ruleIcons = map[string]glyph{
	"firefox": icoFirefox, "chrome": icoChrome, "chromium": icoChrome,
	"brave": icoBrave, "vivaldi": icoGlobe, "opera": icoOpera, "edge": icoEdge,

	"npm": icoNpm, "yarn": icoYarn, "pnpm": icoPackage, "bun": icoBun, "deno": icoDeno,
	"pip": icoPython, "go-build": icoGo, "go-mod": icoGo, "cargo": icoRust,
	"gradle": icoGradle, "maven": icoMaven, "composer": icoComposer,
	"vscode": icoVSCode, "jetbrains": icoJetBrain, "docker-build": icoDocker,

	"thumbnails": icoPictures, "session-logs": icoText, "trash": icoTrash,
	"old-installers": icoDownload, "chat-apps": icoChat, "app-caches": icoApp,
	"snap-caches": icoSnap, "snap-revisions": icoSnap,
	"flatpak-caches": icoFlatpak, "flatpak-unused": icoFlatpak,

	"apt-cache": icoPackage, "apt-autoremove": icoTux, "journal": icoText,
	"rotated-logs": icoText, "crash-reports": icoBug,
}

// ruleIcon returns the icon for a clean rule, or a generic one for rules
// added later (or from the user's own rule file).
func ruleIcon(id string) glyph {
	if g, ok := ruleIcons[id]; ok {
		return g
	}
	return icoFolder
}

// dirIcons maps well-known folder names to what they hold.
var dirIcons = map[string]glyph{
	".git": icoGit, ".config": icoConfig, "snap": icoSnap, ".docker": icoDocker,
	".mozilla": icoFirefox, ".vscode": icoVSCode, ".npm": icoNpm,
	"node_modules": icoNode, ".next": icoNode, ".nuxt": icoNode, ".svelte-kit": icoNode,
	".turbo": icoNode, ".parcel-cache": icoNode, ".yarn": icoYarn, ".bun": icoBun,
	".venv": icoPython, "venv": icoPython, "__pycache__": icoPython,
	".pytest_cache": icoPython, ".mypy_cache": icoPython,
	"go": icoGo, ".cargo": icoRust, ".rustup": icoRust,
	".gradle": icoGradle, ".m2": icoMaven, ".dart_tool": icoDart, ".pub-cache": icoDart,
}

// userDirIcons maps XDG user-dirs keys (see platform.Env.UserDirs) to
// icons, so translated folder names ("Unduhan", "Téléchargements") match.
var userDirIcons = map[string]glyph{
	"DESKTOP": icoDesktop, "DOCUMENTS": icoDocs, "DOWNLOAD": icoDownload,
	"MUSIC": icoMusic, "PICTURES": icoPictures, "VIDEOS": icoFilm,
}

// extIcons maps lower-case file extensions to file kinds.
var extIcons = map[string]glyph{}

func init() {
	kinds := []struct {
		g    glyph
		exts string
	}{
		{icoImage, "png jpg jpeg gif webp bmp svg tif tiff heic heif avif ico raw cr2 nef dng xcf psd kra"},
		{icoVideo, "mp4 mkv webm avi mov wmv flv m4v mpg mpeg 3gp ts"},
		{icoAudio, "mp3 flac wav ogg oga opus m4a aac wma aiff"},
		{icoArchive, "zip tar gz tgz xz txz bz2 tbz2 zst lz4 7z rar"},
		{icoPackage, "deb rpm snap flatpak flatpakref appimage whl jar apk"},
		{icoDisk, "iso img qcow2 vdi vmdk vhd vhdx"},
		{icoPDF, "pdf"},
		{icoDoc, "doc docx odt rtf"},
		{icoSheet, "xls xlsx ods csv tsv"},
		{icoSlides, "ppt pptx odp"},
		{icoText, "txt md log"},
		{icoDatabase, "db sqlite sqlite3 sql"},
		{icoCode, "go py js mjs ts tsx jsx rs c h cc cpp hpp java kt rb php sh bash zsh lua dart swift json yaml yml toml xml html css scss"},
	}
	for _, k := range kinds {
		for _, e := range strings.Fields(k.exts) {
			extIcons[e] = k.g
		}
	}
}

// nodeIcon returns the icon for a file or folder. userDirs are the XDG
// user folders by key, used to spot Downloads, Pictures and so on.
func nodeIcon(path string, isDir bool, userDirs map[string]string) glyph {
	name := filepath.Base(path)
	if !isDir {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
		if g, ok := extIcons[ext]; ok {
			return g
		}
		return icoFile
	}
	for key, dir := range userDirs {
		if g, ok := userDirIcons[key]; ok && dir == path {
			return g
		}
	}
	if g, ok := dirIcons[name]; ok {
		return g
	}
	return icoFolder
}

// icon renders g from the current set with style st, followed by a space,
// or nothing when icons are off. Callers take iconWidth() cells off the
// name for it.
func (sh *shared) icon(g glyph, st lipgloss.Style) string {
	switch sh.icons {
	case IconsNerd:
		return st.Render(g.nerd) + " "
	case IconsEmoji:
		return g.emoji + " " // emoji keep their own colors
	}
	return ""
}

// iconWidth is the number of cells icon takes: the glyph and a space.
func (sh *shared) iconWidth() int {
	switch sh.icons {
	case IconsNerd:
		return 2
	case IconsEmoji:
		return 3
	}
	return 0
}

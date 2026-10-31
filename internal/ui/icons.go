package ui

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// Icons are Nerd Font glyphs (https://www.nerdfonts.com, names from
// glyphnames.json). They are off unless the user turns them on with --icons
// or LEGA_ICONS=nerd: without a Nerd Font they draw as empty boxes.
const (
	icoFolder   = ""          // fa-folder
	icoFile     = ""          // fa-file
	icoImage    = ""          // fa-file_image_o
	icoVideo    = ""          // fa-file_video_o
	icoAudio    = ""          // fa-file_audio_o
	icoArchive  = ""          // fa-file_archive_o
	icoPDF      = ""          // fa-file_pdf_o
	icoDoc      = ""          // fa-file_word_o
	icoSheet    = ""          // fa-file_excel_o
	icoSlides   = ""          // fa-file_powerpoint_o
	icoCode     = ""          // fa-file_code_o
	icoText     = ""          // fa-file_text
	icoDatabase = ""          // fa-database
	icoDisk     = ""          // fa-hdd_o
	icoPackage  = ""          // oct-package
	icoDesktop  = ""          // fa-desktop
	icoDocs     = ""          // fa-file_text_o
	icoDownload = ""          // fa-download
	icoMusic    = ""          // fa-music
	icoPictures = ""          // fa-picture_o
	icoFilm     = ""          // fa-film
	icoConfig   = ""          // fa-cog
	icoTrash    = ""          // fa-trash
	icoBug      = ""          // fa-bug
	icoChat     = ""          // fa-comments
	icoApp      = ""          // fa-cube
	icoTux      = ""          // fa-linux
	icoGlobe    = ""          // fa-globe
	icoFirefox  = ""          // fa-firefox
	icoChrome   = ""          // fa-chrome
	icoOpera    = ""          // fa-opera
	icoEdge     = ""          // fa-edge
	icoGit      = ""          // fa-git
	icoDocker   = ""          // linux-docker
	icoSnap     = ""          // linux-snappy
	icoFlatpak  = ""          // linux-flathub
	icoVSCode   = "\U000f0a1e" // md-microsoft_visual_studio_code
	icoJetBrain = ""          // dev-jetbrains
	icoNode     = ""          // dev-nodejs_small
	icoNpm      = ""          // dev-npm
	icoYarn     = ""          // seti-yarn
	icoBun      = ""          // dev-bun
	icoDeno     = ""          // dev-denojs
	icoPython   = ""          // dev-python
	icoGo       = ""          // seti-go
	icoRust     = ""          // dev-rust
	icoGradle   = ""          // dev-gradle
	icoMaven    = ""          // dev-maven
	icoComposer = ""          // dev-composer
	icoDart     = ""          // dev-dart
)

// ruleIcons maps clean rule IDs to the app or kind of data they clean.
var ruleIcons = map[string]string{
	"firefox": icoFirefox, "chrome": icoChrome, "chromium": icoChrome,
	"brave": icoGlobe, "vivaldi": icoGlobe, "opera": icoOpera, "edge": icoEdge,

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

// ruleIcon returns the glyph for a clean rule, or a generic one for rules
// added later (or from the user's own rule file).
func ruleIcon(id string) string {
	if g, ok := ruleIcons[id]; ok {
		return g
	}
	return icoFolder
}

// dirIcons maps well-known folder names to what they hold.
var dirIcons = map[string]string{
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
var userDirIcons = map[string]string{
	"DESKTOP": icoDesktop, "DOCUMENTS": icoDocs, "DOWNLOAD": icoDownload,
	"MUSIC": icoMusic, "PICTURES": icoPictures, "VIDEOS": icoFilm,
}

// extIcons maps lower-case file extensions to file kinds.
var extIcons = map[string]string{}

func init() {
	kinds := map[string]string{
		icoImage:    "png jpg jpeg gif webp bmp svg tif tiff heic heif avif ico raw cr2 nef dng xcf psd kra",
		icoVideo:    "mp4 mkv webm avi mov wmv flv m4v mpg mpeg 3gp ts",
		icoAudio:    "mp3 flac wav ogg oga opus m4a aac wma aiff",
		icoArchive:  "zip tar gz tgz xz txz bz2 tbz2 zst lz4 7z rar",
		icoPackage:  "deb rpm snap flatpak flatpakref appimage whl jar apk",
		icoDisk:     "iso img qcow2 vdi vmdk vhd vhdx",
		icoPDF:      "pdf",
		icoDoc:      "doc docx odt rtf",
		icoSheet:    "xls xlsx ods csv tsv",
		icoSlides:   "ppt pptx odp",
		icoText:     "txt md log",
		icoDatabase: "db sqlite sqlite3",
		icoCode:     "go py js mjs ts tsx jsx rs c h cc cpp hpp java kt rb php sh bash zsh lua dart swift json yaml yml toml xml html css scss",
	}
	for g, exts := range kinds {
		for _, e := range strings.Fields(exts) {
			extIcons[e] = g
		}
	}
}

// nodeIcon returns the glyph for a file or folder. userDirs are the XDG
// user folders by key, used to spot Downloads, Pictures and so on.
func nodeIcon(path string, isDir bool, userDirs map[string]string) string {
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

// iconCells is how many cells an icon takes on screen, the glyph and the
// space after it. Nerd Font glyphs are one cell wide.
const iconCells = 2

// icon renders glyph g with style st followed by a space, or nothing when
// icons are off. Callers take iconWidth() cells off the name for it.
func (sh *shared) icon(g string, st lipgloss.Style) string {
	if !sh.icons {
		return ""
	}
	return st.Render(g) + " "
}

// iconWidth is the number of cells icon takes.
func (sh *shared) iconWidth() int {
	if !sh.icons {
		return 0
	}
	return iconCells
}

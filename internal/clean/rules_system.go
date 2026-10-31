package clean

import "time"

const month = 30 * 24 * time.Hour

// trashParts are the parts of a trash folder: files, their info, and
// leftovers from interrupted emptying.
var trashParts = []string{"files/*", "info/*", "expunged/*"}

func systemRules() []Rule {
	return []Rule{
		&pathRule{
			info: Info{
				ID: "thumbnails", Name: "Thumbnail cache", Group: GroupSystem,
				Description: "Image previews made by the file manager. They are made again when needed.",
			},
			spots: []spot{{"$CACHE/thumbnails", []string{"*"}}},
		},
		&pathRule{
			info: Info{
				ID: "session-logs", Name: "Old session logs", Group: GroupSystem,
				Description: "Logs from previous desktop sessions.",
			},
			spots: []spot{
				{"~", []string{".xsession-errors.old"}},
				{"$DATA/xorg", []string{"*.log.old"}},
			},
		},
		&pathRule{
			info: Info{
				ID: "trash", Name: "Trash", Group: GroupFiles, Risk: Review,
				Description: "Files you moved to the Trash, on this disk and on mounted drives. They are deleted for good.",
			},
			spots: []spot{
				{"$DATA/Trash", trashParts},
				{"/media/$USER/*/.Trash-$UID", trashParts},
			},
		},
		&pathRule{
			info: Info{
				ID: "old-installers", Name: "Old installers in Downloads", Group: GroupFiles, Risk: Review,
				Description: "Package and disk image files (.deb, .AppImage, .iso, .run, .rpm) not touched for 30 days.",
			},
			spots:     []spot{{"$DOWNLOAD", []string{"*.deb", "*.AppImage", "*.appimage", "*.iso", "*.run", "*.rpm"}}},
			olderThan: month,
			filesOnly: true,
		},
	}
}

// generalRules catch caches no specific rule claimed. They run last.
func generalRules() []Rule {
	return []Rule{
		&pathRule{
			info: Info{
				ID: "snap-caches", Name: "Snap app caches", Group: GroupApps, Risk: Review,
				Description: "Cache folders of snap apps. Apps rebuild them, but may start slower once.",
			},
			spots: []spot{{"~/snap", []string{"*/*/.cache/*"}}},
		},
		&pathRule{
			info: Info{
				ID: "flatpak-caches", Name: "Flatpak app caches", Group: GroupApps, Risk: Review,
				Description: "Cache folders of Flatpak apps. Apps rebuild them, but may start slower once.",
			},
			spots: []spot{{"~/.var/app", []string{"*/cache/*"}}},
		},
		&pathRule{
			info: Info{
				ID: "app-caches", Name: "Other app caches", Group: GroupApps, Risk: Review,
				Description: "Everything else in ~/.cache. Apps should rebuild it, but check the list first.",
			},
			spots: []spot{{"$CACHE", []string{"*"}}},
			skip:  []string{"lega"},
		},
	}
}

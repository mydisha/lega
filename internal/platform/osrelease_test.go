package platform

import (
	"os"
	"strings"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	cases := []struct {
		file      string
		id        string
		pretty    string
		supported bool
	}{
		{"zorin", "zorin", "Zorin OS 18.1", true},
		{"mint", "linuxmint", "Linux Mint 22.1", true},
		{"pop", "pop", "Pop!_OS 24.04 LTS", true},
		{"elementary", "elementary", "elementary OS 8 Circe", true},
		{"debian", "debian", "Debian GNU/Linux 13 (trixie)", true},
		{"fedora", "fedora", "Fedora Linux 42 (Workstation Edition)", false},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			f, err := os.Open("testdata/" + c.file)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			o, err := ParseOSRelease(f)
			if err != nil {
				t.Fatal(err)
			}
			if o.ID != c.id || o.DisplayName() != c.pretty || o.Supported() != c.supported {
				t.Errorf("got id=%q name=%q supported=%v", o.ID, o.DisplayName(), o.Supported())
			}
		})
	}
}

func TestParseOSReleaseOddLines(t *testing.T) {
	in := "# comment\n\nNOEQUALS\nID='ubuntu'\nNAME=\"Say \\\"hi\\\"\"\n"
	o, err := ParseOSRelease(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if o.ID != "ubuntu" || o.Name != `Say "hi"` {
		t.Errorf("got %+v", o)
	}
	if (OSRelease{}).DisplayName() != "Linux" {
		t.Error("empty release should display as Linux")
	}
}

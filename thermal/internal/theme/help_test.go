package theme

import (
	"testing"
)

func TestHelpStylesUsesThemeColors(t *testing.T) {
	for _, name := range []string{"classic", "frappe"} {
		t.Run(name, func(t *testing.T) {
			th, err := Get(name)
			if err != nil {
				t.Fatalf("Get(%q): %v", name, err)
			}
			styles := th.HelpStyles()

			// ShortKey / FullKey use HelpColor.
			if got, want := styles.ShortKey.GetForeground(), th.HelpColor; got != want {
				t.Errorf("ShortKey foreground = %v, want HelpColor %v", got, want)
			}
			if got, want := styles.FullKey.GetForeground(), th.HelpColor; got != want {
				t.Errorf("FullKey foreground = %v, want HelpColor %v", got, want)
			}

			// ShortDesc / FullDesc / separators / ellipsis use DimColor.
			dimStyles := []struct {
				name string
				fg   any
			}{
				{"ShortDesc", styles.ShortDesc.GetForeground()},
				{"ShortSeparator", styles.ShortSeparator.GetForeground()},
				{"FullDesc", styles.FullDesc.GetForeground()},
				{"FullSeparator", styles.FullSeparator.GetForeground()},
				{"Ellipsis", styles.Ellipsis.GetForeground()},
			}
			for _, ds := range dimStyles {
				if ds.fg != th.DimColor {
					t.Errorf("%s foreground = %v, want DimColor %v", ds.name, ds.fg, th.DimColor)
				}
			}
		})
	}
}

package layout

import (
	"os"
	"testing"

	zone "github.com/lrstanley/bubblezone/v2"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

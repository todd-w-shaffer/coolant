package widgets

import "github.com/toddwshaffer/coolant/cc-viz-go/internal/model"

// Widget is the interface all composable UI components implement.
type Widget interface {
	SetSize(width, height int)
	Update(state *model.AppState)
	View() string
}

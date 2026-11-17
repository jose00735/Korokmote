// Package tray shows the notification-area icon and menu.
package tray

// Item is one menu entry. A nil OnClick with an empty Title is a separator.
type Item struct {
	Title    string
	Checked  bool
	Disabled bool
	OnClick  func()
}

// Config configures the tray.
type Config struct {
	Tooltip string
	IconPNG []byte
	Menu    func() []Item // built fresh on every right-click
	OnLeft  func()        // left click
	OnReady func(*Tray)
}

// Separator is a menu separator.
var Separator = Item{}

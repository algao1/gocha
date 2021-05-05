package main

import (
	"regexp"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// InputHandler handles input from tview.InputField.
type InputHandler func(message string)

// hexToTCell converts a hex string to the corresponding TCell.Color.
func hexToTCell(hexStr string) tcell.Color {
	result, _ := strconv.ParseInt(hexStr, 16, 64)
	return tcell.NewHexColor(int32(result))
}

// NewChatBox initializes and returns a 'chatBox' to display incoming
// chat messages.
func NewChatBox(palette map[string]string) *tview.TextView {
	chatBox := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetTextAlign(tview.AlignLeft)

	chatBox.SetBorder(true).
		SetTitle("#general").
		SetTitleColor(hexToTCell(palette["title"])).
		SetTitleAlign(tview.AlignLeft).
		SetBorderColor(hexToTCell(palette["border"])).
		SetBackgroundColor(hexToTCell(palette["background"]))

	return chatBox
}

// NewChatInput initializes and returns a 'chatInput' component
// that handles user inputs and forwards chat messages.
func NewChatInput(palette map[string]string, handler InputHandler) *tview.InputField {
	nLine := regexp.MustCompile(`\\n\s?`)

	// Initialize 'inputField' for user inputs.
	chatInput := tview.NewInputField()
	chatInput.SetFieldBackgroundColor(tcell.ColorBlack).
		SetFieldWidth(0).
		SetFieldTextColor(hexToTCell(palette["text"])).
		SetFieldBackgroundColor(hexToTCell(palette["background"])).
		SetDoneFunc(func(key tcell.Key) {
			// Send and clear inputField.
			if key == tcell.KeyEnter {
				message := nLine.ReplaceAllString(chatInput.GetText(), "\n") + "\n"
				handler(message)
				chatInput.SetText("")
			}
		}).
		SetBorder(true).
		SetBorderColor(hexToTCell(palette["border"])).
		SetBackgroundColor(hexToTCell(palette["background"]))

	return chatInput
}

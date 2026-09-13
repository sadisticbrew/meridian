package render

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Minutes renders n as "65m" or "4h 05m" (week/table aggregates).
func Minutes(n int) string {
	if n < 60 {
		return fmt.Sprintf("%dm", n)
	}
	return fmt.Sprintf("%dh %02dm", n/60, n%60)
}

// MinutesShort renders n as "65m", "3h" or "1h 30m" (per-thing breakdowns).
func MinutesShort(n int) string {
	if n < 60 {
		return fmt.Sprintf("%dm", n)
	}
	if n%60 == 0 {
		return fmt.Sprintf("%dh", n/60)
	}
	return fmt.Sprintf("%dh %02dm", n/60, n%60)
}

// Compact renders whole seconds as "32m" or "1h10m" (single-line CLI output).
func Compact(seconds int) string {
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh%02dm", minutes/60, minutes%60)
}

// Underline repeats the box-drawing dash to the rune width of s.
func Underline(s string) string {
	return strings.Repeat("─", utf8.RuneCountInString(s))
}

// Plural renders "1 review" / "3 reviews".
func Plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// Bar scales minutes to a block bar of at most width cells.
func Bar(minutes, maxMinutes, width int) string {
	if minutes <= 0 || maxMinutes <= 0 {
		return ""
	}
	scaled := minutes * width / maxMinutes
	if scaled < 1 {
		scaled = 1
	}
	return strings.Repeat("█", scaled)
}

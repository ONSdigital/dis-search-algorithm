package ui

import (
	"fmt"
	"os"
)

// Verbose is a global variable that controls the verbosity of the output
var Verbose = false

// Info prints an informational message
func Info(format string, args ...interface{}) {
	fmt.Printf("ℹ️  "+format+"\n", args...)
}

// Success prints a success message
func Success(format string, args ...interface{}) {
	fmt.Printf("✅ "+format+"\n", args...)
}

// Warning prints a warning message
func Warning(format string, args ...interface{}) {
	fmt.Printf("⚠️  "+format+"\n", args...)
}

// Error prints an error message
func Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "❌ "+format+"\n", args...)
}

// Debug prints a debug message (only if verbose)
func Debug(format string, args ...interface{}) {
	if Verbose {
		fmt.Printf("🔍 "+format+"\n", args...)
	}
}

// Section prints a section header
func Section(title string) {
	fmt.Println()
	fmt.Println(repeatChar("=", 60))
	fmt.Printf("  %s\n", title)
	fmt.Println(repeatChar("=", 60))
	fmt.Println()
}

// Celebrate prints a celebration message
func Celebrate(format string, args ...interface{}) {
	fmt.Println()
	fmt.Println(repeatChar("=", 60))
	fmt.Printf("🎉 "+format+"\n", args...)
	fmt.Println(repeatChar("=", 60))
	fmt.Println()
}

// Pluralise returns a string with
// the number and the pluralised term.
func Pluralise(term string, number int) string {
	if number != 1 {
		term += "s"
	}
	return fmt.Sprintf("%d %s", number, term)
}

func repeatChar(char string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += char
	}
	return result
}

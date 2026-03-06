package main

import (
	"syscall"
	"unsafe"
)

// showError displays a Windows message box with the error message.
func showError(msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	msgBox := user32.NewProc("MessageBoxW")

	title, _ := syscall.UTF16PtrFromString("GoClaw")
	text, _ := syscall.UTF16PtrFromString(msg)

	const mbIconError = 0x00000010
	msgBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), mbIconError)
}

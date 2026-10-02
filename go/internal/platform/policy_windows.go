//go:build windows

package platform

import "syscall"

// ClaudePolicyKeyPresent reports whether native's registry policy key exists. Native 2.1.287
// reads managed settings from HKLM and HKCU SOFTWARE\Policies\ClaudeCode (value "Settings";
// measured in the binary 2026-10-02). Any answer other than "not found" counts as present,
// so a key this process cannot read still keeps Clauduct's required confirmations.
func ClaudePolicyKeyPresent() bool {
	path, err := syscall.UTF16PtrFromString(`SOFTWARE\Policies\ClaudeCode`)
	if err != nil {
		return true
	}
	for _, root := range []syscall.Handle{syscall.HKEY_LOCAL_MACHINE, syscall.HKEY_CURRENT_USER} {
		var key syscall.Handle
		switch err := syscall.RegOpenKeyEx(root, path, 0, syscall.KEY_READ, &key); err {
		case nil:
			syscall.RegCloseKey(key)
			return true
		case syscall.ERROR_FILE_NOT_FOUND:
		default:
			return true
		}
	}
	return false
}

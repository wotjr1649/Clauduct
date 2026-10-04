//go:build windows

package platform

import (
	"errors"
	"syscall"
	"unsafe"
)

// maxPolicyBytes bounds one registry policy value. Native caps a policy helper's output at
// 1 MiB; a settings document past that is not one this build reads.
const maxPolicyBytes = 1 << 20

// ClaudePolicy returns native's registry policy documents: value "Settings" (JSON, REG_SZ or
// REG_EXPAND_SZ) under SOFTWARE\Policies\ClaudeCode in HKLM and in HKCU (managed-settings,
// native 2.1.289). An absent key or value is empty. Any other failure is an error, so a
// policy this process cannot read is never taken for no policy.
func ClaudePolicy() (machine, user string, err error) {
	if machine, err = registryString(syscall.HKEY_LOCAL_MACHINE, policyKey, "Settings"); err != nil {
		return "", "", err
	}
	user, err = registryString(syscall.HKEY_CURRENT_USER, policyKey, "Settings")
	return machine, user, err
}

const policyKey = `SOFTWARE\Policies\ClaudeCode`

// registryString reads one REG_SZ or REG_EXPAND_SZ value; an absent key or value is empty.
func registryString(root syscall.Handle, subkey, value string) (string, error) {
	path, err := syscall.UTF16PtrFromString(subkey)
	if err != nil {
		return "", err
	}
	var key syscall.Handle
	switch err := syscall.RegOpenKeyEx(root, path, 0, syscall.KEY_READ, &key); err {
	case nil:
	case syscall.ERROR_FILE_NOT_FOUND:
		return "", nil
	default:
		return "", err
	}
	defer syscall.RegCloseKey(key)
	name, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return "", err
	}
	var kind, size uint32
	switch err := syscall.RegQueryValueEx(key, name, nil, &kind, nil, &size); err {
	case nil:
	case syscall.ERROR_FILE_NOT_FOUND:
		return "", nil
	default:
		return "", err
	}
	if kind != syscall.REG_SZ && kind != syscall.REG_EXPAND_SZ || size > maxPolicyBytes {
		return "", errors.New("unsupported registry policy value")
	}
	if size < 2 {
		return "", nil
	}
	// Rounded up: a value whose byte count is odd must still fit, and the size passed back
	// in is the buffer's, never more.
	buf := make([]uint16, (size+1)/2)
	size = uint32(len(buf) * 2)
	if err := syscall.RegQueryValueEx(key, name, nil, &kind, (*byte)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return "", err
	}
	return syscall.UTF16ToString(buf), nil
}

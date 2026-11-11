//go:build !darwin && !linux

package nativeforms

import "os"

func readSystemFontFile(path string) (data []byte, mapped bool, err error) {
	data, err = os.ReadFile(path)
	return data, false, err
}

package svdrp_test

import "os"

func mkdirAllMode(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}

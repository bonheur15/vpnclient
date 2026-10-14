package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// When started with sudo, files we create in the user's config dir would
// end up owned by root and become read-only for later non-root runs (the
// normal mode once setup has granted OpenVPN access). Hand them back.
func platformInit(dir string)    { chownToSudoUser(dir) }
func platformCleanup(dir string) { chownToSudoUser(dir) }

func chownToSudoUser(dir string) {
	if os.Geteuid() != 0 {
		return
	}
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))
	if err1 != nil || err2 != nil || uid == 0 {
		return
	}
	_ = filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err == nil {
			_ = os.Lchown(p, uid, gid)
		}
		return nil
	})
}

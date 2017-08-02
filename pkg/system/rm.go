package system

import (
	"os"
	"syscall"
	"time"

	"github.com/docker/docker/pkg/mount"
	"github.com/pkg/errors"
)

// EnsureRemoveAll wraps `os.RemoveAll` to check for specific errors that can
// often be remedied.
// Only use `EnsureRemoveAll` if you really want to make every effort to remove
// a directory.
//
// Because of the way `os.Remove` (and by extension `os.RemoveAll`) works, there
// can be a race between reading directory entries and then actually attempting
// to remove everything in the directory.
// These types of errors do not need to be returned since it's ok for the dir to
// be gone we can just retry the remove operation.
//
// This should not return a `os.ErrNotExist` kind of error under any circumstances
func EnsureRemoveAll(dir string) error {
	notExistErr := make(map[string]bool)

	// track retries
	exitOnErr := make(map[string]int)
	maxRetry := 5

	// Attempt to unmount anything beneath this dir first
	mount.RecursiveUnmount(dir)

	for {
		err := os.RemoveAll(dir)
		if err == nil {
			return err
		}

		pe, ok := err.(*os.PathError)
		if !ok {
			return err
		}

		if os.IsNotExist(err) {
			if notExistErr[pe.Path] {
				return err
			}
			notExistErr[pe.Path] = true

			// There is a race where some subdir can be removed but after the parent
			//   dir entries have been read.
			// So the path could be from `os.Remove(subdir)`
			// If the reported non-existent path is not the passed in `dir` we
			// should just retry, but otherwise return with no error.
			if pe.Path == dir {
				return nil
			}
			continue
		}

		if pe.Err != syscall.EBUSY {
			return err
		}

		if mounted, _ := mount.Mounted(pe.Path); mounted {
			if e := mount.Unmount(pe.Path); e != nil {
				if mounted, _ := mount.Mounted(pe.Path); mounted {
					return errors.Wrapf(e, "error while removing %s", dir)
				}
			}
		}

		if exitOnErr[pe.Path] == maxRetry {
			return err
		}
		exitOnErr[pe.Path]++
		time.Sleep(100 * time.Millisecond)
	}
}

// AtomicRemoveAll performs an atomic remove using `EnsureRemoveAll`
// During removal the passed in dir will not be accessible.
func AtomicRemoveAll(dir string) error {
	// best effort to unmount, if this fails (on a mounted fs), the rename/rm will fail below accordingly
	mount.Unmount(dir)

	renamed := dir + "-removing"
	err := os.Rename(dir, renamed)
	switch {
	case os.IsNotExist(err):
		// origin dir does not exist, nothing to do
		return nil
	case os.IsExist(err):
		// Some previous remove failed, check if the origin dir exists -- it should not.
		if _, e := os.Stat(dir); !os.IsNotExist(e) {
			return errors.Wrap(err, "both rename target and origin dir exist")
		}
	default:
		return errors.Wrap(err, "error attempting to rename dir for atomic removal")
	}

	if err := EnsureRemoveAll(renamed); err != nil {
		os.Rename(renamed, dir)
		return err
	}
	return nil
}

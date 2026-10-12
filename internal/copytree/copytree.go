// Package copytree copies a directory tree with everything an OS needs kept: owners,
// modes (setuid too), symlinks, hard links, device nodes, xattrs and times.
package copytree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

type Options struct {
	// Owner maps a source owner to the one written; nil keeps it.
	Owner func(uid, gid uint32) (uint32, uint32)
	// Skip leaves out a path (relative to the source root) and everything under it.
	Skip func(rel string) bool
	// Copied receives the number of bytes copied so far.
	Copied func(total int64)
	// Workers copying file data in parallel (default 8).
	Workers int
}

// Measure returns the bytes and entries Copy will process (hard links count once).
func Measure(src string, skip func(string) bool) (bytes int64, entries int64, err error) {
	seen := map[inodeKey]bool{}
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skip != nil && p != src {
			if rel, _ := filepath.Rel(src, p); skip(rel) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		entries++
		if d.Type().IsRegular() {
			var st unix.Stat_t
			if unix.Lstat(p, &st) == nil {
				k := inodeKey{st.Dev, st.Ino}
				if st.Nlink > 1 && seen[k] {
					return nil
				}
				seen[k] = true
				bytes += st.Size
			}
		}
		return nil
	})
	return
}

type inodeKey struct{ dev, ino uint64 }

type job struct {
	src, dst string
	st       unix.Stat_t
}

type copier struct {
	o      Options
	copied atomic.Int64
	links  map[inodeKey]string
	later  [][2]string // hard links to make once their first name is written
	dirs   []job       // directory metadata, applied deepest first at the end
	jobs   chan job
	mu     sync.Mutex
	err    error
}

func (c *copier) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
}

func (c *copier) failed() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Copy copies src into dst (dst is created if needed and gets src's metadata).
func Copy(src, dst string, o Options) error {
	if o.Workers <= 0 {
		o.Workers = 8
	}
	c := &copier{o: o, links: map[inodeKey]string{}, jobs: make(chan job, 256)}
	var wg sync.WaitGroup
	for range o.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range c.jobs {
				if c.failed() != nil {
					continue
				}
				if err := c.file(j); err != nil {
					c.fail(err)
				}
			}
		}()
	}
	walkErr := c.walk(src, dst)
	close(c.jobs)
	wg.Wait()
	if walkErr != nil {
		return walkErr
	}
	if err := c.failed(); err != nil {
		return err
	}
	for _, l := range c.later {
		if err := os.Link(l[0], l[1]); err != nil {
			return fmt.Errorf("hard link %s: %w", l[1], err)
		}
	}
	for i := len(c.dirs) - 1; i >= 0; i-- {
		if err := c.meta(c.dirs[i], -1); err != nil {
			return err
		}
	}
	return nil
}

func (c *copier) walk(src, dst string) error {
	var root unix.Stat_t
	if err := unix.Lstat(src, &root); err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	if root.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("%s is not a directory", src)
	}
	return c.dir(src, dst, "", root)
}

func (c *copier) dir(src, dst, rel string, st unix.Stat_t) error {
	if err := os.Mkdir(dst, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	c.dirs = append(c.dirs, job{src, dst, st})
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	for _, name := range names {
		if err := c.failed(); err != nil {
			return err
		}
		s, d, r := filepath.Join(src, name), filepath.Join(dst, name), filepath.Join(rel, name)
		if c.o.Skip != nil && c.o.Skip(r) {
			continue
		}
		var st unix.Stat_t
		if err := unix.Lstat(s, &st); err != nil {
			return fmt.Errorf("%s: %w", s, err)
		}
		if err := c.entry(s, d, r, st); err != nil {
			return err
		}
	}
	return nil
}

func (c *copier) entry(src, dst, rel string, st unix.Stat_t) error {
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		return c.dir(src, dst, rel, st)
	case unix.S_IFREG:
		if st.Nlink > 1 {
			k := inodeKey{st.Dev, st.Ino}
			if first, ok := c.links[k]; ok {
				c.later = append(c.later, [2]string{first, dst})
				return nil
			}
			c.links[k] = dst
		}
		c.jobs <- job{src, dst, st}
		return nil
	case unix.S_IFLNK:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		os.Remove(dst)
		if err := os.Symlink(target, dst); err != nil {
			return err
		}
		return c.meta(job{src, dst, st}, -1)
	case unix.S_IFCHR, unix.S_IFBLK, unix.S_IFIFO:
		os.Remove(dst)
		if err := unix.Mknod(dst, st.Mode, int(st.Rdev)); err != nil {
			return fmt.Errorf("mknod %s: %w", dst, err)
		}
		return c.meta(job{src, dst, st}, -1)
	}
	return nil // sockets are runtime-only
}

func (c *copier) file(j job) error {
	in, err := os.Open(j.src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(j.dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	for {
		n, err := io.CopyN(out, in, 8<<20)
		if n > 0 && c.o.Copied != nil {
			c.o.Copied(c.copied.Add(n))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			out.Close()
			return fmt.Errorf("copy %s: %w", j.src, err)
		}
	}
	if err := c.meta(j, int(out.Fd())); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// meta sets owner, mode, xattrs and times; fd is the open file or -1 to use the path.
func (c *copier) meta(j job, fd int) error {
	uid, gid := j.st.Uid, j.st.Gid
	if c.o.Owner != nil {
		uid, gid = c.o.Owner(uid, gid)
	}
	link := j.st.Mode&unix.S_IFMT == unix.S_IFLNK
	var err error
	if fd >= 0 {
		err = unix.Fchown(fd, int(uid), int(gid))
	} else {
		err = unix.Lchown(j.dst, int(uid), int(gid))
	}
	if err != nil {
		return fmt.Errorf("chown %s: %w", j.dst, err)
	}
	if err := copyXattrs(j.src, j.dst, fd, link); err != nil {
		return err
	}
	// mode after chown: chown clears setuid/setgid
	if !link {
		mode := j.st.Mode & 0o7777
		if fd >= 0 {
			err = unix.Fchmod(fd, mode)
		} else {
			err = unix.Chmod(j.dst, mode)
		}
		if err != nil {
			return fmt.Errorf("chmod %s: %w", j.dst, err)
		}
	}
	ts := []unix.Timespec{j.st.Atim, j.st.Mtim}
	if fd >= 0 {
		err = unix.UtimesNanoAt(fd, "", ts, unix.AT_EMPTY_PATH)
		if err != nil {
			err = unix.Futimes(fd, []unix.Timeval{unix.NsecToTimeval(unix.TimespecToNsec(ts[0])), unix.NsecToTimeval(unix.TimespecToNsec(ts[1]))})
		}
	} else {
		err = unix.UtimesNanoAt(unix.AT_FDCWD, j.dst, ts, unix.AT_SYMLINK_NOFOLLOW)
	}
	if err != nil {
		return fmt.Errorf("times %s: %w", j.dst, err)
	}
	return nil
}

func copyXattrs(src, dst string, fd int, link bool) error {
	size, err := unix.Llistxattr(src, nil)
	if err != nil || size == 0 {
		return nil // no xattrs, or the source fs has none
	}
	buf := make([]byte, size)
	size, err = unix.Llistxattr(src, buf)
	if err != nil {
		return nil
	}
	for _, name := range splitNames(buf[:size]) {
		if name == "security.selinux" {
			continue // labels belong to the build host, Zainium has no SELinux policy
		}
		n, err := unix.Lgetxattr(src, name, nil)
		if err != nil {
			continue
		}
		val := make([]byte, n)
		if n, err = unix.Lgetxattr(src, name, val); err != nil {
			continue
		}
		if fd >= 0 {
			err = unix.Fsetxattr(fd, name, val[:n], 0)
		} else {
			err = unix.Lsetxattr(dst, name, val[:n], 0)
		}
		if err != nil && !(link && (errors.Is(err, unix.EPERM) || errors.Is(err, unix.ENOTSUP))) {
			return fmt.Errorf("xattr %s on %s: %w", name, dst, err)
		}
	}
	return nil
}

func splitNames(b []byte) []string {
	var out []string
	start := 0
	for i, c := range b {
		if c == 0 {
			if i > start {
				out = append(out, string(b[start:i]))
			}
			start = i + 1
		}
	}
	return out
}

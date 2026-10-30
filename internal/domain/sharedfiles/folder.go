package sharedfiles

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// File is one shared file: its name in the folder (with / between folders,
// on every system), and what is in it.
type File struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Dir is the shared files' folder in a worktree. Git ignores it: it is
// under .agentry, Agentry's own.
func Dir(worktree string) string { return filepath.Join(worktree, ".agentry", "files") }

// EpicFolder is where a story's worktree has its epic's files, to read:
// what changes there isn't kept.
const EpicFolder = "epic"

// CleanName is name as a shared file's name, or why it can't be one.
func CleanName(name string) (string, error) {
	name = path.Clean(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	switch {
	case name == "." || name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || name == "..":
		return "", fmt.Errorf("%q isn't a name for a shared file: give a path inside the folder, such as data/sales.csv", name)
	case len(name) > 400:
		return "", errors.New("a shared file's name is at most 400 characters")
	}
	return name, nil
}

// Manifest is every file in the worktree's folder but its epic's, with
// what is in each.
func Manifest(worktree string) ([]File, error) {
	root := Dir(worktree)
	var files []File
	err := filepath.WalkDir(root, func(full string, entry fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, full)
		name := filepath.ToSlash(relative)
		if entry.IsDir() {
			if name == EpicFolder {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil // links and the like aren't shared
		}
		hash, size, err := HashOf(full)
		if err != nil {
			return err
		}
		files = append(files, File{Name: name, Hash: hash, Size: size})
		return nil
	})
	return files, err
}

// Deliver makes the worktree's folder hold exactly files: what is missing
// or different comes from blobs, and what isn't one of them goes.
func Deliver(worktree string, files []File, blobs Blobs) error {
	root := Dir(worktree)
	want := map[string]File{}
	for _, f := range files {
		name, err := CleanName(f.Name)
		if err != nil {
			return err
		}
		want[name] = f
		full := filepath.Join(root, filepath.FromSlash(name))
		if hash, _, err := HashOf(full); err == nil && hash == f.Hash {
			continue
		}
		if err := fetchBlob(blobs, f.Hash, full); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	have, err := everything(root)
	if err != nil {
		return err
	}
	for _, name := range have {
		if _, kept := want[name]; !kept {
			os.Remove(filepath.Join(root, filepath.FromSlash(name)))
		}
	}
	return nil
}

func fetchBlob(blobs Blobs, hash, full string) error {
	body, err := blobs.Open(hash)
	if err != nil {
		return err
	}
	defer body.Close()
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	temporary := full + ".agentry-part"
	out, err := os.Create(temporary)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, body)
	out.Close()
	if err != nil {
		os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, full)
}

// everything is the name of every file in root, epic's too.
func everything(root string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(root, func(full string, entry fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err == nil && !entry.IsDir() {
			relative, _ := filepath.Rel(root, full)
			names = append(names, filepath.ToSlash(relative))
		}
		return err
	})
	return names, err
}

// Upload sends the named files in the worktree's folder to blobs.
func Upload(worktree string, names []string, blobs Blobs) ([]File, error) {
	var sent []File
	for _, name := range names {
		name, err := CleanName(name)
		if err != nil {
			return nil, err
		}
		f, err := put(filepath.Join(Dir(worktree), filepath.FromSlash(name)), name, blobs)
		if err != nil {
			return nil, err
		}
		sent = append(sent, f)
	}
	return sent, nil
}

func put(full, name string, blobs Blobs) (File, error) {
	file, err := os.Open(full)
	if err != nil {
		return File{}, err
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() > MaxSize {
		return File{}, fmt.Errorf("%s: %w", name, ErrTooBig)
	}
	hash, size, err := blobs.Put(file)
	return File{Name: name, Hash: hash, Size: size}, err
}

// ErrOutside is a path outside where it may be taken from without asking.
var ErrOutside = errors.New("outside the worker's folder")

// Fetch sends the file, or every file in the folder, at full to blobs, named
// as under as. With a boundary, full must be inside it (ErrOutside).
func Fetch(full, boundary, as string, blobs Blobs) ([]File, error) {
	full, err := filepath.Abs(full)
	if err != nil {
		return nil, err
	}
	if boundary != "" {
		if inside, err := filepath.Rel(boundary, full); err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
			return nil, ErrOutside
		}
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if as == "" {
		as = filepath.Base(full)
	}
	if as, err = CleanName(as); err != nil {
		return nil, err
	}
	if !info.IsDir() {
		f, err := put(full, as, blobs)
		return []File{f}, err
	}
	var sent []File
	err = filepath.WalkDir(full, func(each string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !entry.Type().IsRegular() {
			return err
		}
		relative, _ := filepath.Rel(full, each)
		f, err := put(each, as+"/"+filepath.ToSlash(relative), blobs)
		sent = append(sent, f)
		return err
	})
	return sent, err
}

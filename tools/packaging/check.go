package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/macho"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The release check: before a release is published, the files from every
// machine that built them are together in release/. It checks that they are
// the release's files and that every program in them was built from the
// tag's commit with no changes, then writes release/SHA256SUMS and prints
// the command that publishes them. It doesn't publish them itself.
//
//	go run ./tools/packaging check [vX.Y.Z]   (make release-check [VERSION=vX.Y.Z])
//
// It doesn't see a stale page: one built from an older ui/src and committed.

// releaseFile is one file of a release, with the programs in its folder
// Agentry.
type releaseFile struct {
	name     string
	programs []string
}

func runCheck(args []string) error {
	tag, err := releaseTag(args)
	if err != nil {
		return err
	}
	commit, err := tagCommit(tag)
	if err != nil {
		return err
	}
	problems := checkRelease("release", tag, commit)
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, problem)
		}
		return fmt.Errorf("release %s isn't ready: %d problems", tag, len(problems))
	}
	if err := writeSums("release", releaseFiles(tag)); err != nil {
		return err
	}
	fmt.Printf("Release %s is ready: %d files, all built from %s with no changes. Wrote release/SHA256SUMS.\n", tag, len(releaseFiles(tag)), commit)
	fmt.Println("Publish it with:")
	fmt.Println(publishCommand(tag, releaseFiles(tag)))
	return nil
}

// releaseTag is the release's version: the one named, or the tag at HEAD.
func releaseTag(args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	tag, err := git("describe", "--tags", "--exact-match")
	if err != nil {
		return "", fmt.Errorf("HEAD has no tag of its own: check out the release's tag, or name it with VERSION=vX.Y.Z")
	}
	return tag, nil
}

// tagCommit is the commit the tag points to, annotated or not. A branch or
// any other revision is refused.
func tagCommit(tag string) (string, error) {
	commit, err := git("rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s isn't a tag: name the release's tag, such as VERSION=v0.1.0", tag)
	}
	return commit, nil
}

func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// releaseFiles are the 9 files of the release version: the server's zips
// and the three desktop apps.
func releaseFiles(version string) []releaseFile {
	var files []releaseFile
	for _, p := range platforms {
		files = append(files, releaseFile{serverZip(version, p), serverPrograms(p)})
	}
	var windows, mac []string
	for _, program := range desktopPrograms {
		windows = append(windows, program+".exe")
		mac = append(mac, macProgramsFolder+"/"+program)
	}
	return append(files,
		releaseFile{linuxDesktopFile(version), desktopPrograms},
		releaseFile{windowsDesktopFile(version), windows},
		releaseFile{macDesktopFile(version), mac},
	)
}

// checkRelease checks the release files of version in dir, every program
// in them against commit, and that no build of a tree with changes or of an
// untagged commit is there. It returns every problem it finds.
func checkRelease(dir, version, commit string) []string {
	var problems []string
	for _, file := range releaseFiles(version) {
		path := filepath.Join(dir, file.name)
		if _, err := os.Stat(path); err != nil {
			problems = append(problems, path+": missing")
			continue
		}
		for _, problem := range checkPrograms(path, file.programs, commit) {
			problems = append(problems, path+": "+problem)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return append(problems, err.Error())
	}
	stray := strayBuild(version)
	for _, entry := range entries {
		if stray.MatchString(entry.Name()) {
			problems = append(problems, filepath.Join(dir, entry.Name())+": not a build of the tagged commit; remove it")
		}
	}
	return problems
}

// strayBuild matches the files of version that make package or make desktop
// names for a tree with changes (-dirty) or for commits after the tag
// (-N-gHASH).
func strayBuild(version string) *regexp.Regexp {
	return regexp.MustCompile(`^agentry(-desktop)?-` + regexp.QuoteMeta(version) + `-(dirty|\d+-g[0-9a-f]+)`)
}

// checkPrograms checks each of programs, in the folder Agentry in the zip or
// .tar.gz at path, against commit.
func checkPrograms(path string, programs []string, commit string) []string {
	wanted := map[string]bool{}
	for _, program := range programs {
		wanted["Agentry/"+program] = true
	}
	var problems []string
	err := eachFile(path, func(name string, r io.Reader) error {
		if !wanted[name] {
			return nil
		}
		delete(wanted, name)
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		for _, problem := range programProblems(data, commit) {
			problems = append(problems, name+": "+problem)
		}
		return nil
	})
	if err != nil {
		return append(problems, err.Error())
	}
	for _, program := range programs {
		if wanted["Agentry/"+program] {
			problems = append(problems, "Agentry/"+program+": missing")
		}
	}
	return problems
}

// eachFile calls use with the name and contents of each file in the zip or
// .tar.gz at path.
func eachFile(path string, use func(name string, r io.Reader) error) error {
	if strings.HasSuffix(path, ".zip") {
		z, err := zip.OpenReader(path)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			r, err := f.Open()
			if err != nil {
				return err
			}
			err = use(f.Name, r)
			r.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	t := tar.NewReader(gz)
	for {
		header, err := t.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := use(header.Name, t); err != nil {
			return err
		}
	}
}

// programProblems checks the program data against commit: each arch of a
// universal Mac program, since buildinfo reads only the first.
func programProblems(data []byte, commit string) []string {
	r := bytes.NewReader(data)
	fat, err := macho.NewFatFile(r)
	if err != nil {
		return buildProblems(r, commit)
	}
	defer fat.Close()
	var problems []string
	for _, arch := range fat.Arches {
		slice := io.NewSectionReader(r, int64(arch.Offset), int64(arch.Size))
		for _, problem := range buildProblems(slice, commit) {
			problems = append(problems, arch.Cpu.String()+": "+problem)
		}
	}
	return problems
}

// buildProblems checks that the program r was built from commit, with no
// changes in the tree.
func buildProblems(r io.ReaderAt, commit string) []string {
	info, err := buildinfo.Read(r)
	if err != nil {
		return []string{"no Go build info: " + err.Error()}
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	revision, ok := settings["vcs.revision"]
	if !ok {
		return []string{"no commit in its build info (built outside git, or with -buildvcs=false)"}
	}
	var problems []string
	if revision != commit {
		problems = append(problems, "built from "+revision+", not the tag's "+commit)
	}
	if settings["vcs.modified"] != "false" {
		problems = append(problems, "built from a tree with changes (changed or untracked files)")
	}
	return problems
}

// writeSums writes dir/SHA256SUMS for files, as sha256sum writes it, sorted
// by name, so sha256sum -c checks the files downloaded beside it.
func writeSums(dir string, files []releaseFile) error {
	var names []string
	for _, file := range files {
		names = append(names, file.name)
	}
	sort.Strings(names)
	var sums strings.Builder
	for _, name := range names {
		sum, err := fileSum(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(&sums, "%x  %s\n", sum, name)
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums.String()), 0o644)
}

func fileSum(path string) ([]byte, error) {
	in, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, in); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

// publishCommand is the gh command that publishes the release tag with its
// files and SHA256SUMS, from the repo's root.
func publishCommand(tag string, files []releaseFile) string {
	command := []string{"gh", "release", "create", tag}
	for _, file := range files {
		command = append(command, "release/"+file.name)
	}
	command = append(command, "release/SHA256SUMS", "--repo", "agent3-labs/agentry", "--verify-tag", "--title", tag, `--notes "What changed"`)
	return strings.Join(command, " ")
}

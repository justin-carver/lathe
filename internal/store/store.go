package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devenjarvis/lathe/internal/config"
	"github.com/devenjarvis/lathe/internal/verify"
)

func Store(srcPath string, withVerify bool) (*Tutorial, error) {
	slug := filepath.Base(strings.TrimSuffix(srcPath, string(filepath.Separator)))

	tutorialsDir, err := config.TutorialsDir()
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(tutorialsDir, slug)
	if err := copyDir(srcPath, destDir); err != nil {
		return nil, fmt.Errorf("copy tutorial: %w", err)
	}

	parts, series := detectParts(destDir)
	status := StatusVerified
	if withVerify {
		status = StatusVerifying
	}

	t := &Tutorial{
		Slug:    slug,
		Title:   SlugToTitle(slug),
		Topic:   slug,
		Created: time.Now().UTC(),
		Status:  status,
		Series:  series,
		Parts:   parts,
	}

	if err := WriteMetadata(destDir, t); err != nil {
		return nil, err
	}

	if withVerify {
		if err := verify.SpawnVerifier(slug, destDir); err != nil {
			return nil, fmt.Errorf("spawn verifier: %w", err)
		}
	}

	return t, nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func detectParts(dir string) ([]string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var parts []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "part-") && strings.HasSuffix(e.Name(), ".md") {
			parts = append(parts, e.Name())
		}
	}
	sort.Strings(parts)
	return parts, len(parts) > 0
}

func Delete(slug string) error {
	if slug == "" || slug == "." || slug == ".." || strings.ContainsAny(slug, `/\`) {
		return fmt.Errorf("invalid slug: %q", slug)
	}
	tutorialsDir, err := config.TutorialsDir()
	if err != nil {
		return err
	}
	target := filepath.Join(tutorialsDir, slug)
	if !strings.HasPrefix(target, tutorialsDir+string(filepath.Separator)) {
		return fmt.Errorf("invalid slug: %q", slug)
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("tutorial %q not found", slug)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a tutorial directory: %q", slug)
	}
	return os.RemoveAll(target)
}

func SlugToTitle(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

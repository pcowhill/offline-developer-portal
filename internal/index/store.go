package index

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Manifest describes a generated corpus. It is written last, so a corpus
// directory without a manifest is incomplete.
type Manifest struct {
	Format        string           `json:"format"`
	FormatVersion int              `json:"format_version"`
	Generator     string           `json:"generator"`
	GeneratedAt   time.Time        `json:"generated_at"`
	DocumentCount int              `json:"document_count"`
	SearchIndex   string           `json:"search_index"`
	DocumentsPath string           `json:"documents_path"`
	Sources       []SourceManifest `json:"sources"`
}

// SourceManifest records provenance and crawl statistics for one source.
type SourceManifest struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	StartURLs     []string  `json:"start_urls"`
	DocumentCount int       `json:"document_count"`
	IndexedAt     time.Time `json:"indexed_at"`
	Status        string    `json:"status"` // "ok", "partial", "failed" or "kept" (re-used from a previous run)
	PagesFetched  int       `json:"pages_fetched"`
	PagesSkipped  int       `json:"pages_skipped"`
	Errors        int       `json:"errors"`
	ErrorSamples  []string  `json:"error_samples,omitempty"`
	RobotsTxt     bool      `json:"respects_robots_txt"`
}

// DocumentPath returns the path of a document relative to the corpus root,
// using forward slashes (as used in URLs).
func DocumentPath(id string) string {
	return DocumentsDir + "/" + id[:2] + "/" + id + ".json"
}

// Write writes a complete corpus into dir, which must not already exist.
// Documents and the search index are written first and the manifest last.
func Write(dir string, m *Manifest, docs []*Document) error {
	if err := os.MkdirAll(filepath.Join(dir, DocumentsDir), 0o755); err != nil {
		return err
	}
	for _, d := range docs {
		p := filepath.Join(dir, filepath.FromSlash(DocumentPath(d.ID)))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := writeJSON(p, d, false); err != nil {
			return fmt.Errorf("writing document %s: %w", d.URL, err)
		}
	}
	si := BuildSearchIndex(docs)
	if err := writeJSON(filepath.Join(dir, SearchIndexFile), si, false); err != nil {
		return fmt.Errorf("writing search index: %w", err)
	}
	m.Format = CorpusFormat
	m.FormatVersion = FormatVersion
	m.DocumentCount = len(docs)
	m.SearchIndex = SearchIndexFile
	m.DocumentsPath = DocumentsDir + "/"
	sort.SliceStable(m.Sources, func(i, j int) bool { return m.Sources[i].Name < m.Sources[j].Name })
	return writeJSON(filepath.Join(dir, ManifestFile), m, true)
}

func writeJSON(path string, v any, indent bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if indent {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		f.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(bufio.NewReader(f)).Decode(v)
}

// ReadManifest reads dir/manifest.json and checks its format.
func ReadManifest(dir string) (*Manifest, error) {
	var m Manifest
	if err := readJSON(filepath.Join(dir, ManifestFile), &m); err != nil {
		return nil, err
	}
	if m.Format != CorpusFormat {
		return nil, fmt.Errorf("%s is not an offline developer portal corpus", dir)
	}
	if m.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("corpus format version %d is not supported (expected %d); re-run indexing", m.FormatVersion, FormatVersion)
	}
	return &m, nil
}

// ReadSearchIndex reads dir/search-index.json.
func ReadSearchIndex(dir string) (*SearchIndex, error) {
	var si SearchIndex
	if err := readJSON(filepath.Join(dir, SearchIndexFile), &si); err != nil {
		return nil, err
	}
	if si.Format != SearchIndexFormat || si.FormatVersion != FormatVersion {
		return nil, errors.New("unsupported search index format; re-run indexing")
	}
	return &si, nil
}

// ReadDocument reads one document by id.
func ReadDocument(dir, id string) (*Document, error) {
	if len(id) < 3 {
		return nil, errors.New("invalid document id")
	}
	var d Document
	if err := readJSON(filepath.Join(dir, filepath.FromSlash(DocumentPath(id))), &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ReadDocumentsForSources loads every stored document belonging to one of
// the given sources. It is used to keep sources that are not being
// re-crawled.
func ReadDocumentsForSources(dir string, sources map[string]bool) ([]*Document, error) {
	var docs []*Document
	root := filepath.Join(dir, DocumentsDir)
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || filepath.Ext(p) != ".json" {
			return nil
		}
		var d Document
		if err := readJSON(p, &d); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if sources[d.SourceID] {
			docs = append(docs, &d)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	return docs, nil
}

// Replace atomically (as far as the filesystem allows) replaces the corpus
// at dst with the freshly written corpus at tmp. The previous corpus is
// moved aside first and removed only after the swap succeeded.
func Replace(tmp, dst string) error {
	backup := dst + ".previous"
	_ = os.RemoveAll(backup)
	hadOld := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, backup); err != nil {
			return fmt.Errorf("could not move the old corpus aside (%v); if the portal server is running, stop it and try again", err)
		}
		hadOld = true
	}
	if err := os.Rename(tmp, dst); err != nil {
		if hadOld {
			_ = os.Rename(backup, dst)
		}
		return fmt.Errorf("could not move the new corpus into place: %w", err)
	}
	if hadOld {
		_ = os.RemoveAll(backup)
	}
	return nil
}

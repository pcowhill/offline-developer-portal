// Package indexer runs the "index" phase: it reads sources.yaml, crawls
// every configured source and writes a complete offline corpus.
package indexer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pcowhill/offline-developer-portal/internal/config"
	"github.com/pcowhill/offline-developer-portal/internal/crawler"
	"github.com/pcowhill/offline-developer-portal/internal/index"
)

// Options configure an indexing run.
type Options struct {
	ConfigPath string
	CorpusDir  string
	// Only limits crawling to these source ids. Other sources that are
	// still configured keep their documents from the existing corpus.
	Only      []string
	Version   string
	Out       io.Writer
	Transport http.RoundTripper
	Now       func() time.Time
}

// Summary describes a finished run.
type Summary struct {
	Manifest *index.Manifest
	Results  []*crawler.Result
}

// Run executes the index phase. The existing corpus is replaced only if
// the run completes and produced at least one document.
func Run(ctx context.Context, opts Options) (*Summary, error) {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	logf := func(format string, args ...any) { fmt.Fprintf(out, format+"\n", args...) }

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	selected := map[string]bool{}
	for _, id := range opts.Only {
		if cfg.Source(id) == nil {
			return nil, fmt.Errorf("unknown source id %q (configured ids: %s)", id, strings.Join(sourceIDs(cfg), ", "))
		}
		selected[id] = true
	}
	crawlAll := len(selected) == 0

	printPlan(logf, cfg, selected, opts.ConfigPath, opts.CorpusDir)

	// Sources that are not being re-crawled keep their previous documents.
	var kept []*index.Document
	keptManifest := map[string]index.SourceManifest{}
	if !crawlAll {
		if old, err := index.ReadManifest(opts.CorpusDir); err == nil {
			keep := map[string]bool{}
			for _, s := range old.Sources {
				if !selected[s.ID] && cfg.Source(s.ID) != nil {
					keep[s.ID] = true
					keptManifest[s.ID] = s
				}
			}
			if len(keep) > 0 {
				kept, err = index.ReadDocumentsForSources(opts.CorpusDir, keep)
				if err != nil {
					return nil, fmt.Errorf("reading existing corpus: %w", err)
				}
				logf("Keeping %d previously indexed documents from %d other source(s).", len(kept), len(keep))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			logf("Note: existing corpus could not be read (%v); only the selected sources will be included.", err)
		}
	}

	userAgent := crawler.UserAgent(opts.Version, cfg.Settings.UserAgentContact)
	summary := &Summary{}
	manifest := &index.Manifest{
		Generator: "offline-docs " + opts.Version,
	}
	docs := append([]*index.Document(nil), kept...)

	for i := range cfg.Sources {
		src := &cfg.Sources[i]
		if !crawlAll && !selected[src.ID] {
			if m, ok := keptManifest[src.ID]; ok {
				m.Name = src.Name
				manifest.Sources = append(manifest.Sources, m)
			}
			continue
		}
		logf("")
		logf("=== Indexing %q (%s), up to %d pages ===", src.Name, src.ID, src.MaxPages)
		res, err := crawler.Crawl(ctx, crawler.Options{
			Config:    cfg,
			UserAgent: userAgent,
			Log:       logf,
			Transport: opts.Transport,
			Now:       now,
		}, src)
		if err != nil {
			if ctx.Err() != nil {
				return nil, errors.New("indexing was interrupted; the existing corpus was left unchanged")
			}
			return nil, err
		}
		summary.Results = append(summary.Results, res)
		docs = append(docs, res.Documents...)
		status := "ok"
		switch {
		case len(res.Documents) == 0:
			status = "failed"
		case res.Errors > 0:
			status = "partial"
		}
		manifest.Sources = append(manifest.Sources, index.SourceManifest{
			ID:            src.ID,
			Name:          src.Name,
			StartURLs:     append([]string(nil), src.StartURLs...),
			DocumentCount: len(res.Documents),
			IndexedAt:     res.FinishedAt.UTC().Truncate(time.Second),
			Status:        status,
			PagesFetched:  res.PagesFetched,
			PagesSkipped:  res.PagesSkipped,
			Errors:        res.Errors,
			ErrorSamples:  res.ErrorSamples,
			RobotsTxt:     res.RobotsTxt,
		})
		logf("=== %s: %d documents, %d pages fetched, %d skipped, %d errors, %d out-of-scope links ignored (%s) ===",
			src.ID, len(res.Documents), res.PagesFetched, res.PagesSkipped, res.Errors, res.OutOfScope,
			res.FinishedAt.Sub(res.StartedAt).Round(time.Second))
	}

	if len(docs) == 0 {
		return nil, errors.New("no documents were indexed, so the existing corpus was left unchanged; check the errors above (network access, URLs, boundaries, robots.txt)")
	}

	manifest.GeneratedAt = now().UTC().Truncate(time.Second)
	tmp := opts.CorpusDir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return nil, fmt.Errorf("removing stale temporary directory %s: %w", tmp, err)
	}
	if err := index.Write(tmp, manifest, docs); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("writing corpus: %w", err)
	}
	if err := index.Replace(tmp, opts.CorpusDir); err != nil {
		return nil, err
	}
	summary.Manifest = manifest

	logf("")
	logf("Corpus written to %s", opts.CorpusDir)
	logf("  %d documents from %d source(s), generated %s", manifest.DocumentCount, len(manifest.Sources), manifest.GeneratedAt.Format(time.RFC3339))
	for _, s := range manifest.Sources {
		line := fmt.Sprintf("  - %-24s %5d documents  [%s]", s.ID, s.DocumentCount, s.Status)
		if s.Errors > 0 {
			line += fmt.Sprintf("  %d errors", s.Errors)
		}
		logf("%s", line)
	}
	return summary, nil
}

func sourceIDs(cfg *config.Config) []string {
	ids := make([]string, len(cfg.Sources))
	for i, s := range cfg.Sources {
		ids[i] = s.ID
	}
	return ids
}

func printPlan(logf func(string, ...any), cfg *config.Config, selected map[string]bool, cfgPath, corpusDir string) {
	logf("Offline Developer Portal - indexing")
	logf("  configuration: %s", cfgPath)
	logf("  output corpus: %s", corpusDir)
	logf("  concurrency %d, %s between requests to the same host, %s timeout",
		cfg.Settings.Concurrency, cfg.RequestDelay(), cfg.RequestTimeout())
	logf("")
	logf("The crawler only follows links inside the boundaries listed below.")
	total := 0
	for _, s := range cfg.Sources {
		if len(selected) > 0 && !selected[s.ID] {
			continue
		}
		total += s.MaxPages
		logf("  [%s] %s", s.ID, s.Name)
		for _, u := range s.StartURLs {
			logf("      start:   %s", u)
		}
		for _, p := range s.AllowedPrefixes {
			logf("      prefix:  %s", p)
		}
		for _, h := range s.AllowedHosts {
			logf("      host:    %s (entire host)", h)
		}
		for _, p := range s.Include {
			logf("      include: %s", p)
		}
		for _, p := range s.Exclude {
			logf("      exclude: %s", p)
		}
		depth := "unlimited"
		if s.MaxDepth > 0 {
			depth = fmt.Sprint(s.MaxDepth)
		}
		logf("      max_pages: %d, max_depth: %s, robots.txt: %s", s.MaxPages, depth, onOff(cfg.RespectsRobots(&s)))
		if s.MaxPages > config.LargeSourceWarning {
			logf("      WARNING: max_pages is large (%d). Make sure this is intended.", s.MaxPages)
		}
		if len(s.AllowedHosts) > 0 {
			logf("      NOTE: allowed_hosts permits every page on the host; prefer allowed_prefixes for large sites.")
		}
	}
	logf("  At most %d page requests in total.", total)
}

func onOff(b bool) string {
	if b {
		return "respected"
	}
	return "IGNORED (explicit override)"
}

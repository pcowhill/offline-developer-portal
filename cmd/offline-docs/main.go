// Command offline-docs crawls configured documentation sites into a local
// corpus (index) and serves the static Offline Developer Portal (serve).
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pcowhill/offline-developer-portal/internal/config"
	"github.com/pcowhill/offline-developer-portal/internal/index"
	"github.com/pcowhill/offline-developer-portal/internal/indexer"
	"github.com/pcowhill/offline-developer-portal/internal/server"
)

// version is set at build time with -ldflags "-X main.version=v0.1.0".
var version = "dev"

const usage = `Offline Developer Portal

Usage:
  offline-docs index    [--config sources.yaml] [--corpus corpus] [--source ID]...
  offline-docs serve    [--dist dist] [--corpus corpus] [--port 8080] [--host 127.0.0.1] [--open]
  offline-docs validate [--config sources.yaml]
  offline-docs search   [--corpus corpus] [--source ID] [--limit 10] QUERY...
  offline-docs version

Phase 1 ("index") crawls the sources in sources.yaml and writes a static
corpus. Phase 2 ("serve") serves the portal and the corpus on localhost and
needs no network access to the original sites.

Run "offline-docs <command> -h" for the options of a command.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		if runtime.GOOS == "windows" {
			// Probably double-clicked in Explorer: keep the window open.
			fmt.Fprint(os.Stderr, "\nOn Windows, use index-windows.cmd and serve-windows.cmd instead.\nPress Enter to close this window.")
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		}
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "index":
		err = runIndex(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	case "validate":
		err = runValidate(os.Args[2:])
	case "search":
		err = runSearch(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("offline-docs", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "\nERROR: %v\n", err)
		os.Exit(1)
	}
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			*m = append(*m, s)
		}
	}
	return nil
}

// defaultPath prefers name in the working directory, then next to the
// executable (so the release folder works from any working directory).
func defaultPath(name string) string {
	if _, err := os.Stat(name); err == nil {
		return name
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, cand := range []string{filepath.Join(dir, name), filepath.Join(dir, "..", name)} {
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
		}
	}
	return name
}

func runIndex(args []string) error {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "path to sources.yaml (default: ./sources.yaml)")
	corpus := fs.String("corpus", "", "output corpus directory (default: ./corpus)")
	var only multiFlag
	fs.Var(&only, "source", "only re-index this source id (repeatable); other sources keep their existing documents")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cfgPath == "" {
		*cfgPath = defaultPath("sources.yaml")
	}
	if *corpus == "" {
		*corpus = filepath.Join(filepath.Dir(*cfgPath), "corpus")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()
	_, err := indexer.Run(ctx, indexer.Options{
		ConfigPath: *cfgPath,
		CorpusDir:  *corpus,
		Only:       only,
		Version:    version,
		Out:        os.Stdout,
	})
	if err != nil {
		return err
	}
	fmt.Printf("\nIndexing finished in %s. Start the portal with serve-windows.cmd or ./serve-linux.sh.\n", time.Since(start).Round(time.Second))
	return nil
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "path to sources.yaml (default: ./sources.yaml)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cfgPath == "" {
		*cfgPath = defaultPath("sources.yaml")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	total := 0
	for _, s := range cfg.Sources {
		total += s.MaxPages
		fmt.Printf("  %-24s max_pages=%-5d %s\n", s.ID, s.MaxPages, s.Name)
	}
	fmt.Printf("%s is valid: %d source(s), at most %d page requests.\n", *cfgPath, len(cfg.Sources), total)
	return nil
}

func runSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	corpus := fs.String("corpus", "", "corpus directory (default: ./corpus)")
	limit := fs.Int("limit", 10, "maximum number of results")
	var sources multiFlag
	fs.Var(&sources, "source", "restrict to this source id (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *corpus == "" {
		*corpus = defaultPath("corpus")
	}
	query := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(query) == "" {
		return errors.New("no query given")
	}
	si, err := index.ReadSearchIndex(*corpus)
	if err != nil {
		return fmt.Errorf("reading corpus %s: %w (run the index command first)", *corpus, err)
	}
	results := si.Search(query, index.SearchOptions{Sources: sources, Limit: *limit})
	if len(results) == 0 {
		fmt.Println("No results.")
		return nil
	}
	for i, r := range results {
		fmt.Printf("%2d. %s  [%s]  score %.2f\n    %s\n", i+1, r.Doc.Title, r.Doc.SourceID, r.Score, r.Doc.URL)
	}
	return nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dist := fs.String("dist", "", "directory containing the built portal (default: ./dist)")
	corpus := fs.String("corpus", "", "corpus directory (default: ./corpus)")
	host := fs.String("host", "127.0.0.1", "address to listen on; the default only accepts connections from this computer")
	port := fs.Int("port", 8080, "port to listen on (if busy and not set explicitly, the next free port is used)")
	open := fs.Bool("open", false, "open the portal in the default web browser")
	if err := fs.Parse(args); err != nil {
		return err
	}
	portSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			portSet = true
		}
	})
	if *dist == "" {
		*dist = defaultPath("dist")
	}
	if *corpus == "" {
		*corpus = defaultPath("corpus")
	}
	if _, err := os.Stat(filepath.Join(*dist, "index.html")); err != nil {
		return fmt.Errorf("the portal files were not found in %q (expected %s). Run this from the extracted release folder", *dist, filepath.Join(*dist, "index.html"))
	}

	corpusNote := ""
	if m, err := index.ReadManifest(*corpus); err == nil {
		corpusNote = fmt.Sprintf("%d documents from %d source(s), generated %s", m.DocumentCount, len(m.Sources), m.GeneratedAt.Local().Format("2006-01-02 15:04"))
	} else if errors.Is(err, os.ErrNotExist) {
		corpusNote = "NOT FOUND - run index-windows.cmd or ./index-linux.sh first"
	} else {
		corpusNote = "unreadable: " + err.Error()
	}

	ln, err := listen(*host, *port, !portSet)
	if err != nil {
		return err
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port
	displayHost := *host
	if displayHost == "127.0.0.1" || displayHost == "::1" || displayHost == "" || displayHost == "0.0.0.0" {
		displayHost = "localhost"
	}
	url := fmt.Sprintf("http://%s/", net.JoinHostPort(displayHost, strconv.Itoa(actualPort)))

	srv := &http.Server{
		Handler:           server.Handler(server.Options{DistDir: *dist, CorpusDir: *corpus}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Println()
	fmt.Println("  Offline Developer Portal is running.")
	fmt.Println()
	fmt.Printf("      Open:  %s\n", url)
	if displayHost == "localhost" {
		fmt.Printf("        or:  http://127.0.0.1:%d/\n", actualPort)
	}
	fmt.Println()
	fmt.Printf("  Portal files: %s\n", absOr(*dist))
	fmt.Printf("  Corpus:       %s (%s)\n", absOr(*corpus), corpusNote)
	if *host != "127.0.0.1" && *host != "localhost" && *host != "::1" {
		fmt.Printf("  NOTE: listening on %s; other computers on the network may be able to connect.\n", *host)
	}
	fmt.Println()
	fmt.Println("  Leave this window open while using the portal. Press Ctrl+C to stop.")
	fmt.Println()

	if *open {
		openBrowser(url)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		fmt.Println("Stopping...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func listen(host string, port int, tryNext bool) (net.Listener, error) {
	var lastErr error
	attempts := 1
	if tryNext {
		attempts = 20
	}
	for i := 0; i < attempts; i++ {
		p := port + i
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err == nil {
			if i > 0 {
				fmt.Printf("  Port %d is busy; using port %d instead.\n", port, p)
			}
			return ln, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("could not listen on %s port %d: %v (is another copy already running? try --port 8090)", host, port, lastErr)
}

func absOr(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return
		}
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("  (Could not open a browser automatically: %v. Open the address above manually.)\n", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}

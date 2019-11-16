package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	ln "github.com/GeertJohan/go.linenoise"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var bulkMoveCommand = cli.Command{
	Name:  "bk",
	Usage: "bulk move files with your editor",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "files,f",
			Usage: "move files only",
		},
		cli.BoolFlag{
			Name:  "hidden",
			Usage: "show hidden files",
		},
	},
	Action: func(clix *cli.Context) (err error) {

		path := clix.Args().First()
		if path == "" {
			if path, err = filepath.Abs("."); err != nil {
				return errors.Wrap(err, "cannot get absolute path")
			}
		}
		w := &walker{
			base: path,
		}
		if clix.Bool("files") {
			w.handlers = append(w.handlers, skipDirs)
		}
		if !clix.Bool("hidden") {
			w.handlers = append(w.handlers, skipHidden)
		}
		if err := filepath.Walk(path, w.walk); err != nil {
			return errors.Wrapf(err, "walking %s", path)
		}
		tmp, err := tempFile(path, w.results)
		if err != nil {
			return err
		}
		if err := startEditor(tmp); err != nil {
			return errors.Wrap(err, "running editor")
		}
		moves, err := createMoveMap(path, w.results, tmp)
		if err != nil {
			return err
		}
		var i int
		for src, dest := range moves {
			if shouldSkipDisplay(src, dest) {
				continue
			}
			rel, err := display(path, src)
			if err != nil {
				return errors.Wrap(err, "display path")
			}
			fmt.Printf("%s -> %s\n", rel, dest)
			i++
		}
		if i == 0 {
			fmt.Println("no edits made")
			return nil
		}
		ln.SetMultiline(true)
		answer, err := ln.Line("Commit? ")
		if err != nil {
			return errors.Wrap(err, "readline")
		}
		if strings.TrimSpace(strings.ToUpper(answer)) == "YES" {
			for src, dest := range moves {
				if shouldSkipDisplay(src, dest) {
					continue
				}
				if err := os.Rename(src, dest); err != nil {
					fmt.Printf("error %s: %s -> %d\n", err, src, dest)
				}
			}
		}
		return nil
	},
}

func shouldSkipDisplay(src, dest string) bool {
	if dest == "" {
		return true
	}
	if src == dest {
		return true
	}
	return false
}

func tempFile(path string, results []*extInfo) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", errors.Wrap(err, "create tmp file")
	}
	defer f.Close()

	for _, r := range results {
		rel, err := display(path, r.Path)
		if err != nil {
			return "", errors.Wrap(err, "get display path")
		}
		if _, err := fmt.Fprintln(f, rel); err != nil {
			return "", errors.Wrap(err, "write path to file")
		}
	}
	return f.Name(), nil
}

func startEditor(path string) error {
	editor := os.Getenv("EDITOR")
	cmd := exec.Command(editor, path)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

const skipMoveToken = '#'

func display(base, path string) (string, error) {
	return filepath.Rel(base, path)
}

func createMoveMap(base string, results []*extInfo, path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", path)
	}
	defer f.Close()

	var (
		i   int
		out = make(map[string]string)
		s   = bufio.NewScanner(f)
	)
	for s.Scan() {
		if err := s.Err(); err != nil {
			return nil, errors.Wrap(err, "scan error")
		}
		line := s.Text()
		if strings.TrimSpace(line) == "" {
			return nil, errors.New("unable to continue with empty line")
		}
		if line[0] == skipMoveToken {
			out[results[i].Path] = ""
		}
		if !filepath.IsAbs(line) {
			line = filepath.Join(base, line)
		}
		out[results[i].Path] = line
		i++
	}
	if len(out) != len(results) {
		return nil, errors.New("lengths do not match")
	}
	return out, nil
}

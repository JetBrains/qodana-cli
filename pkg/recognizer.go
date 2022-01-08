package pkg

import (
	"bytes"
	"github.com/go-enry/go-enry/v2"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func RecognizeDirLanguages(projectPath string) (map[string]int, error) {
	const limitKb = 64
	out := make(map[string]int, 0)
	err := filepath.Walk(projectPath, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		if f.Mode().IsDir() && !f.Mode().IsRegular() {
			return nil
		}

		relpath, err := filepath.Rel(projectPath, path)
		if err != nil {
			return nil
		}

		if relpath == "." {
			return nil
		}

		if f.IsDir() {
			relpath = relpath + "/"
		}
		if enry.IsVendor(relpath) || enry.IsDotFile(relpath) ||
			enry.IsDocumentation(relpath) || enry.IsConfiguration(relpath) ||
			enry.IsGenerated(relpath, nil) {
			if f.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		if f.IsDir() {
			return nil
		}

		content, err := readFile(path, limitKb)
		if err != nil {
			return nil
		}

		if enry.IsGenerated(relpath, content) {
			return nil
		}

		language := enry.GetLanguage(filepath.Base(path), content)
		if language == enry.OtherLanguage {
			return nil
		}

		if enry.GetLanguageType(language) != enry.Programming {
			return nil
		}

		out[language] += 1
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func readFile(path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return ioutil.ReadFile(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			log.Print(err)
		}
	}()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if limit > 0 && size > limit {
		size = limit
	}
	buf := bytes.NewBuffer(nil)
	buf.Grow(int(size))
	_, err = io.Copy(buf, io.LimitReader(f, limit))
	return buf.Bytes(), err
}

func readIdeaFolder(project string) []string {
	var linters []string
	var files []string
	root := project + "/.idea"
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return linters
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		if filepath.Ext(file) == ".iml" {
			iml, err := ioutil.ReadFile(file)
			if err != nil {
				log.Fatal(err)
			}
			text := string(iml)
			if strings.Contains(text, "JAVA_MODULE") {
				linters = append(linters, "jetbrains/qodana-jvm")
			}
			if strings.Contains(text, "PYTHON_MODULE") {
				linters = append(linters, "jetbrains/qodana-python")
			}
			if strings.Contains(text, "WEB_MODULE") {
				xml, err := ioutil.ReadFile(project + "/.idea/workspace.xml")
				if err != nil {
					log.Fatal(err)
				}
				workspace := string(xml)
				if strings.Contains(workspace, "PhpWorkspaceProjectConfiguration") {
					linters = append(linters, "jetbrains/qodana-php")
				} else {
					linters = append(linters, "jetbrains/qodana-js")
				}
			}
		}
	}
	return linters
}

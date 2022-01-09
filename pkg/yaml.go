package pkg

import (
	"bytes"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"path/filepath"
)

// QodanaYaml A standard qodana.yaml (or qodana.yml) format for Qodana configuration.
// https://github.com/JetBrains/qodana-profiles/blob/master/schemas/qodana-yaml-1.0.json
type QodanaYaml struct { // TODO: support full qodana.yaml schema
	// The qodana.yaml version of this log file.
	Version string `yaml:"version,omitempty"`

	// Linters to run.
	Linters []string `yaml:"linters"`

	// The profile configuration for Qodana analysis.
	Profile Profile `yaml:"profile,omitempty"`

	// Number of problems to fail the analysis (to exit from Qodana with code 255).
	FailThreshold int `yaml:"failThreshold,omitempty"`

	// The exclude property to disable the wanted checks on the wanted paths.
	Exclude []Exclude `yaml:"exclude,omitempty"`

	// The include property to enable the wanted checks.
	Include []Include `yaml:"include,omitempty"`
}

// Profile A profile is some template set of checks to run with Qodana analysis.
type Profile struct {
	Name string `yaml:"name"`
	// Path string `yaml:"path"` TODO: support multiple profiles types
}

// Exclude A check id to disable.
type Exclude struct {
	// The name of check to exclude.
	Name string `yaml:"name"`

	// Relative to the project root path to disable analysis.
	Paths []string `yaml:"paths,omitempty"`
}

// Include A check id to enable.
type Include struct {
	// The name of check to exclude.
	Name string `yaml:"name"`
}

func GetQodanaYaml(path string) *QodanaYaml {
	q := &QodanaYaml{}
	yamlFile, err := ioutil.ReadFile(filepath.Join(path, "qodana.yaml"))
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, q)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return q
}

func (q *QodanaYaml) excludeDotQodana() {
	excluded := false
	for i, exclude := range q.Exclude {
		if exclude.Name == "All" {
			if !Contains(exclude.Paths, ".qodana") {
				exclude.Paths = append(exclude.Paths, ".qodana/")
				q.Exclude[i] = exclude
				excluded = true
			}
		}
	}
	if !excluded {
		q.Exclude = append(q.Exclude, Exclude{Name: "All", Paths: []string{".qodana/"}})
	}
}

func WriteQodanaYaml(path string, linters []string) {
	q := GetQodanaYaml(path)
	q.Linters = linters
	q.excludeDotQodana()

	var b bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&b)
	yamlEncoder.SetIndent(2)
	err := yamlEncoder.Encode(&q)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(filepath.Join(path, "qodana.yaml"), b.Bytes(), 0644)
	if err != nil {
		log.Fatalf("Marshal: %v", err)
	}
}

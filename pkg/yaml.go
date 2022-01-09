package pkg

import (
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"path/filepath"
)

type QodanaYaml struct {
	Version string          `yaml:"version"`
	Linters []string        `yaml:"linters"`
	Exclude []QodanaExclude `yaml:"exclude"`
}

type QodanaExclude struct {
	Name  string   `yaml:"name"`
	Paths []string `yaml:"paths"`
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

func WriteQodanaYaml(path string, linters []string) {
	q := &QodanaYaml{
		Version: "1.0",
		Linters: linters,
		Exclude: []QodanaExclude{{
			Name: "All",
			Paths: []string{
				".qodana/",
			},
		}},
	}
	yamlFile, err := yaml.Marshal(q)
	if err != nil {
		log.Fatalf("yamlFile.Write err   #%v ", err)
	}
	err = ioutil.WriteFile(filepath.Join(path, "qodana.yaml"), yamlFile, 0644)
	if err != nil {
		log.Fatalf("Marshal: %v", err)
	}
}

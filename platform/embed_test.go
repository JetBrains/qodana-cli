package platform

import (
	"github.com/spf13/pflag"
	"os"
	"testing"
)

func TestMount(t *testing.T) {
	linterOpts := &TestOptions{}
	options := &QodanaOptions{
		LinterSpecific: linterOpts,
	}
	defer umount()
	mount(options)

	mountInfo := *linterOpts.GetMountInfo()
	if mountInfo.Converter == "" {
		t.Error("mount() failed")
	}

	list := []string{mountInfo.Converter, mountInfo.Fuser, mountInfo.BaselineCli}
	// TODO: should be per-linter test as well
	for _, v := range mountInfo.CustomTools {
		list = append(list, v)
	}

	for _, p := range list {
		_, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				t.Error("Unpacking failed")
			}
		}
	}
}

type TestOptions struct{}

func (TestOptions) AddFlags(_ *pflag.FlagSet) {}

func (TestOptions) GetMountInfo() *MountInfo {
	return &MountInfo{}
}

func (TestOptions) MountTools(_ string, _ string, _ *QodanaOptions) (map[string]string, error) {
	return make(map[string]string), nil
}

func (TestOptions) GetInfo(_ *QodanaOptions) *LinterInfo {
	return &LinterInfo{}
}

func (TestOptions) Setup(_ *QodanaOptions) error {
	return nil
}

func (TestOptions) RunAnalysis(_ *QodanaOptions) error {
	return nil
}
